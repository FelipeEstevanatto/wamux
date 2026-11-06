package whatsmeow_service

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	localmedia "github.com/felipeestevanatto/wamux/pkg/media"
	message_content "github.com/felipeestevanatto/wamux/pkg/message/content"
	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
	"github.com/felipeestevanatto/wamux/pkg/utils"
)

// seenMessageTTL is how long a message id is remembered. The Signal ratchet already
// refuses a replayed ciphertext, so this only guards what reaches us as a new
// delivery of a message handled moments ago.
const seenMessageTTL = time.Hour

// firstDelivery reports whether this is the first time the message was seen, so a
// repeat fires no webhook or chatbot. The key uses the canonical chat, because the
// same message can be addressed by LID or by phone number.
func (mycli *MyClient) firstDelivery(evt *events.Message) bool {
	if mycli.processedMessages == nil {
		return true
	}
	chat := CanonicalChatJID(context.Background(), mycli.WAClient, evt.Info.Chat)
	key := fmt.Sprintf("%s_%s_%t_%s", mycli.userID, evt.Info.ID, evt.Info.IsFromMe, chat)
	return mycli.processedMessages.Add(key, true, seenMessageTTL) == nil
}

// tombstoneRevoked applies a revoke to the stored original. revoke is the row of the
// revoke itself, which supplies the fields when the original was never stored.
func (mycli *MyClient) tombstoneRevoked(revoke message_model.Message, targetID string, evt *events.Message) {
	chats := []string{revoke.ChatJid}
	if raw := evt.Info.Chat.ToNonAD().String(); raw != revoke.ChatJid {
		chats = append(chats, raw)
	}
	if chat, err := types.ParseJID(revoke.ChatJid); err == nil {
		if alt, ok := AlternateChatJID(context.Background(), mycli.WAClient, chat); ok {
			chats = append(chats, alt.String())
		}
	}

	row := revoke
	row.MessageID = targetID
	row.TextContent, row.QuotedMessageID = "", ""
	applied, err := mycli.messageRepository.MarkMessageDeleted(row, chats, !evt.Info.IsGroup)
	if err != nil {
		mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to mark %s as deleted: %v", mycli.userID, targetID, err)
		return
	}
	if !applied {
		mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Ignored a revoke of %s from outside its chat", mycli.userID, targetID)
		return
	}
	if err := localmedia.Remove(mycli.userID, targetID); err != nil {
		mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Failed to remove the media of %s: %v", mycli.userID, targetID, err)
	}
}

// ForgetSentMessage drops a revoked message from whatsmeow's retry cache. Otherwise a
// recipient's retry receipt would be answered by sending the deleted content again.
// With nothing cached the receipt is not answered, which is what a revoke wants.
func ForgetSentMessage(client *whatsmeow.Client, id types.MessageID, chats ...types.JID) {
	if client == nil {
		return
	}
	ctx := context.Background()
	for _, chat := range chats {
		// An entry with no message reads as a cache miss.
		_ = client.DangerousInternals().AddRecentMessage(ctx, chat, id, nil, nil)
		if alt, ok := AlternateChatJID(ctx, client, chat); ok {
			_ = client.DangerousInternals().AddRecentMessage(ctx, alt, id, nil, nil)
		}
	}
}

// storeHistorySync writes the messages of a history batch to the history. Media is
// not fetched: the row keeps the name, size and preview, as WhatsApp sent them.
func (mycli *MyClient) storeHistorySync(data *waHistorySync.HistorySync) {
	rows := mycli.historyRows(data)
	if len(rows) == 0 {
		return
	}
	if err := mycli.messageRepository.InsertHistoryMessages(rows); err != nil {
		mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to store history sync: %v", mycli.userID, err)
		return
	}
	mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Stored %d messages from history sync", mycli.userID, len(rows))
	mycli.markHistoryRead(rows)
}

// markHistoryRead keeps recovered messages from showing as unread: the phone already
// had them, and a new install would otherwise start with every old message unread.
func (mycli *MyClient) markHistoryRead(rows []message_model.Message) {
	newest := make(map[string]string)
	for _, row := range rows {
		if row.Timestamp > newest[row.ChatJid] {
			newest[row.ChatJid] = row.Timestamp
		}
	}
	for chat, at := range newest {
		if err := mycli.messageRepository.MarkChatsRead(mycli.userID, []string{chat}, at); err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Failed to mark recovered history as read: %v", mycli.userID, err)
		}
	}
}

// chatReadElsewhere follows a chat read on the phone, so the manager's badge clears.
func (mycli *MyClient) chatReadElsewhere(evt *events.MarkChatAsRead) {
	if !evt.Action.GetRead() || evt.FromFullSync || !mycli.config.DatabaseSaveMessages {
		return
	}
	ctx := context.Background()
	chat := CanonicalChatJID(ctx, mycli.WAClient, evt.JID)
	chats := []string{chat.String()}
	if alt, ok := AlternateChatJID(ctx, mycli.WAClient, chat); ok {
		chats = append(chats, alt.String())
	}
	at := evt.Timestamp.Format("2006-01-02 15:04:05")
	if err := mycli.messageRepository.MarkChatsRead(mycli.userID, chats, at); err != nil {
		mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Failed to mark a chat as read: %v", mycli.userID, err)
	}
}

func (mycli *MyClient) historyRows(data *waHistorySync.HistorySync) []message_model.Message {
	ctx := context.Background()
	oldest := mycli.historyCutoff()
	var rows []message_model.Message
	for _, conversation := range data.GetConversations() {
		rawChat, err := types.ParseJID(conversation.GetID())
		if err != nil || rawChat == types.StatusBroadcastJID {
			continue // Status posts expire in a day, so they are not history.
		}
		chat := CanonicalChatJID(ctx, mycli.WAClient, rawChat)
		for _, item := range conversation.GetMessages() {
			web := item.GetMessage()
			if web.GetMessage() == nil {
				continue // System notices and already revoked messages have no content.
			}
			evt, err := mycli.WAClient.ParseWebMessage(rawChat, web)
			if err != nil {
				continue
			}
			kind := utils.GetMessageType(evt.Message)
			if kind == "ignore" || kind == "revoke" || kind == "edit" || evt.Info.Timestamp.Before(oldest) {
				continue
			}
			rows = append(rows, historyRow(ctx, mycli, chat, evt, web))
		}
	}
	return rows
}

// historyCutoff is the oldest message worth storing: the retention job would delete
// anything older on its next run. Zero means messages are kept forever.
func (mycli *MyClient) historyCutoff() time.Time {
	if mycli.config.MessageRetentionDays <= 0 {
		return time.Time{}
	}
	return time.Now().AddDate(0, 0, -mycli.config.MessageRetentionDays)
}

func historyRow(ctx context.Context, mycli *MyClient, chat types.JID, evt *events.Message, web *waWeb.WebMessageInfo) message_model.Message {
	preview := message_content.Thumbnail(evt.Message)
	summary := message_content.Summarize(evt.Message)
	sender := CanonicalChatJID(ctx, mycli.WAClient, evt.Info.Sender)
	return message_model.Message{
		MessageID:       evt.Info.ID,
		InstanceId:      mycli.userID,
		Timestamp:       evt.Info.Timestamp.Format("2006-01-02 15:04:05"),
		Status:          historyStatus(web),
		Source:          chat.User,
		ChatJid:         chat.String(),
		SenderJid:       sender.String(),
		MessageType:     summary.Type,
		TextContent:     summary.Text,
		QuotedMessageID: summary.QuotedID,
		IsFromMe:        evt.Info.IsFromMe,
		FileName:        summary.FileName,
		FileSize:        summary.FileSize,
		IsForwarded:     summary.Forwarded,
		PageCount:       summary.PageCount,
		FileThumbnail:   preview,
	}
}

func historyStatus(web *waWeb.WebMessageInfo) string {
	if !web.GetKey().GetFromMe() {
		return "Received"
	}
	switch web.GetStatus() {
	case waWeb.WebMessageInfo_READ, waWeb.WebMessageInfo_PLAYED:
		return "Read"
	case waWeb.WebMessageInfo_DELIVERY_ACK:
		return "Delivered"
	}
	return "Sent"
}

// appStateName is the kind of an app-state change. The rest of the index names the
// chat or contact it applies to, which does not belong in a log.
func appStateName(evt *events.AppState) string {
	if len(evt.Index) == 0 {
		return "unknown"
	}
	return evt.Index[0]
}
