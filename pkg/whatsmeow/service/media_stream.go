package whatsmeow_service

import (
	"context"
	"os"
	"time"

	localmedia "github.com/felipeestevanatto/wamux/pkg/media"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

// streamDownloadTimeout allows for WhatsApp's largest attachments, which the
// in-memory path's five minutes would not.
const streamDownloadTimeout = 30 * time.Minute

// mediaSource is an attachment to stream to disk.
type mediaSource struct {
	media     whatsmeow.DownloadableMessage
	extension string
	mimeType  string
}

// streamableMedia returns the message's attachment when it is larger than limit,
// which is the size that may be held in memory and embedded as base64. Stickers
// never qualify: they are small, and are converted to PNG in memory.
func streamableMedia(msg *waE2E.Message, limit int64) *mediaSource {
	for _, m := range []*waE2E.Message{msg, msg.GetAssociatedChildMessage().GetMessage()} {
		if m == nil {
			continue
		}
		if img := m.GetImageMessage(); img != nil && int64(img.GetFileLength()) > limit {
			return &mediaSource{media: img, extension: ".jpg", mimeType: "image/jpeg"}
		}
		if audio := m.GetAudioMessage(); audio != nil && int64(audio.GetFileLength()) > limit {
			return &mediaSource{media: audio, extension: ".ogg", mimeType: "audio/ogg"}
		}
		if doc := m.GetDocumentMessage(); doc != nil && int64(doc.GetFileLength()) > limit {
			return &mediaSource{media: doc, extension: getExtensionFromMimeType(doc.GetMimetype()), mimeType: doc.GetMimetype()}
		}
		if video := m.GetVideoMessage(); video != nil && int64(video.GetFileLength()) > limit {
			return &mediaSource{media: video, extension: ".mp4", mimeType: "video/mp4"}
		}
	}
	return nil
}

// downloadToFile is the real download for streamMedia. whatsmeow verifies the MAC
// and SHA-256 from the file, so nothing is lost over the in-memory path.
func (mycli *MyClient) downloadToFile(source *mediaSource) func(context.Context, *os.File) error {
	return func(ctx context.Context, file *os.File) error {
		return mycli.WAClient.DownloadToFile(ctx, source.media, file)
	}
}

// streamedMedia reports where a streamed file ended up.
type streamedMedia struct {
	// url is what the event should carry: the object store's URL if there is one,
	// else the API path of the local copy. Empty when the file was not kept.
	url string
	// objectURL is set when an object store has the file, and local when the
	// local store does.
	objectURL string
	local     bool
}

// streamMedia downloads a large attachment to a file and hands it to whichever
// stores want it. Memory use stays flat whatever the size. A file nothing would
// keep is not downloaded at all.
func (mycli *MyClient) streamMedia(messageID string, source *mediaSource, download func(context.Context, *os.File) error) streamedMedia {
	logger := mycli.loggerWrapper.GetLogger(mycli.userID)
	var result streamedMedia

	storage, delivery := mycli.service.MediaStorageFor(mycli.Instance)
	toObjectStore := storage != nil && (delivery == "s3" || delivery == "both")
	toLocal := mycli.config.MediaLocalStore && localmedia.Configured()
	if !toObjectStore && !toLocal {
		logger.LogWarn("[%s] Media too large to embed and no store is configured; not downloading - ID: %s", mycli.userID, messageID)
		return result
	}

	file, err := localmedia.CreateTemp()
	if err != nil {
		logger.LogError("[%s] Could not create a file for media %s: %v", mycli.userID, messageID, err)
		return result
	}
	// Removing a path Adopt already moved is a harmless no-op.
	defer os.Remove(file.Name())
	defer file.Close()

	ctx, cancel := context.WithTimeout(context.Background(), streamDownloadTimeout)
	defer cancel()

	start := time.Now()
	if err := download(ctx, file); err != nil {
		logger.LogError("[%s] Failed to download media %s: %v", mycli.userID, messageID, err)
		return result
	}
	info, err := file.Stat()
	if err != nil {
		logger.LogError("[%s] Could not read downloaded media %s: %v", mycli.userID, messageID, err)
		return result
	}
	logger.LogInfo("[%s] Media streamed to disk - ID: %s, Size: %d bytes, Duration: %v", mycli.userID, messageID, info.Size(), time.Since(start))

	if toObjectStore {
		if _, err := file.Seek(0, 0); err != nil {
			logger.LogError("[%s] Could not rewind media %s: %v", mycli.userID, messageID, err)
		} else if url, err := storage.StoreReader(ctx, file, info.Size(), messageID+source.extension, source.mimeType); err != nil {
			logger.LogError("[%s] Failed to store media %s in S3/Minio: %v", mycli.userID, messageID, err)
		} else {
			result.objectURL, result.url = url, url
		}
	}

	if toLocal {
		if err := localmedia.Adopt(mycli.userID, messageID, file.Name()); err != nil {
			logger.LogWarn("[%s] Failed to store media locally - ID: %s: %v", mycli.userID, messageID, err)
		} else {
			result.local = true
			if result.url == "" {
				result.url = localmedia.URLPath(messageID)
			}
		}
	}
	return result
}
