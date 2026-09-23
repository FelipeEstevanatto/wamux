package user_service

// Save a contact to the device addressbook.
//
// WhatsApp syncs the contact list across devices via APP STATE (the same
// channel as mute/pin/archive): a "contact"-index mutation on the
// critical_unblock_low patch — which whatsmeow itself applies back into
// Store.Contacts on receipt (appstate.go, IndexContact → PutContactName).
// The API also exposes the READ side (GET /user/contacts); this is the WRITE
// side: build the ContactAction (FullName/FirstName + SaveOnPrimaryAddressbook,
// which asks the primary phone to also store it in the SYSTEM addressbook) and
// send it with the official primitive Client.SendAppState. After the resync
// SendAppState triggers, the contact shows up in GET /user/contacts.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	waSyncAction "go.mau.fi/whatsmeow/proto/waSyncAction"
	"google.golang.org/protobuf/proto"
)

type SaveContactStruct struct {
	// Destination number (with country code).
	Number string `json:"number" example:"5582988898565"`
	// Full name saved for the contact.
	FullName string `json:"fullName" example:"John Doe"`
	// First name (optional). When empty, the first word of fullName is used.
	FirstName string `json:"firstName,omitempty" example:"John"`
	// When true (default), sync with the primary phone's addressbook (equivalent
	// to the "Sync contact to phone" toggle in WhatsApp Web).
	SaveOnPhone *bool `json:"saveOnPhone,omitempty"`
}

// contactMutationVersion is the static version of the "contact" index
// mutation (each index has its own — mute=2, pin=5, …; reference
// implementations use 2 for contact). A wrong value fails CLEANLY in
// SendAppState (the server rejects the patch), with no side effects.
const contactMutationVersion = 2

func (u *userService) SaveContact(data *SaveContactStruct, instance *instance_model.Instance) error {
	client, err := u.ensureClientConnected(instance.Id)
	if err != nil {
		return err
	}

	number := strings.TrimSpace(data.Number)
	fullName := strings.TrimSpace(data.FullName)
	if number == "" || fullName == "" {
		return errors.New("number and fullName are required")
	}

	// ParseJID (via CreateJID) also normalizes BR/MX numbers; CanonicalJID strips
	// the "+" prefix so the JID matches the format of the other contacts
	// (556284875027@s.whatsapp.net). With the "+" the app state is accepted but
	// the primary device does not write it into the system addressbook.
	jid, ok := utils.ParseJID(number)
	if !ok {
		return errors.New("invalid phone number")
	}
	jid = utils.CanonicalJID(jid)

	firstName := strings.TrimSpace(data.FirstName)
	if firstName == "" {
		firstName = strings.Fields(fullName)[0]
	}

	// By default sync with the phone addressbook (WhatsApp Web toggle).
	saveOnPhone := true
	if data.SaveOnPhone != nil {
		saveOnPhone = *data.SaveOnPhone
	}

	patch := appstate.PatchInfo{
		Type: appstate.WAPatchCriticalUnblockLow, // the patch that carries the contact list
		Mutations: []appstate.MutationInfo{{
			Index:   []string{appstate.IndexContact, jid.String()},
			Version: contactMutationVersion,
			Value: &waSyncAction.SyncActionValue{
				ContactAction: &waSyncAction.ContactAction{
					FullName:                 proto.String(fullName),
					FirstName:                proto.String(firstName),
					SaveOnPrimaryAddressbook: proto.Bool(saveOnPhone),
				},
			},
		}},
	}

	err = client.SendAppState(context.Background(), patch)
	if err != nil && (strings.Contains(err.Error(), "conflict") || strings.Contains(err.Error(), "LTHash")) {
		// The critical_unblock_low app state desynced (409/LTHash). whatsmeow's
		// incremental recovery does not fix it; force a FULL SYNC (drop the local
		// version, re-download the server snapshot) and retry once.
		u.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] app state conflict on savecontact, forcing full sync of critical_unblock_low", instance.Id)
		if ferr := client.FetchAppState(context.Background(), appstate.WAPatchCriticalUnblockLow, true, false); ferr != nil {
			// Full sync failed too (server snapshot does not verify): deep
			// corruption. Last resort: ask the primary phone to resend the
			// collection (fatal recovery). It is async — the phone responds and
			// whatsmeow rebuilds in the background; the caller should retry in a
			// few seconds.
			u.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] full sync failed (%v); requesting fatal recovery from primary device", instance.Id, ferr)
			recMsg := whatsmeow.BuildAppStateRecoveryRequest(appstate.WAPatchCriticalUnblockLow)
			if _, serr := client.SendPeerMessage(context.Background(), recMsg); serr != nil {
				return fmt.Errorf("contacts app state corrupted and failed to request recovery from the phone: %w", serr)
			}
			return errors.New("the contacts app state was corrupted; recovery was requested from the primary phone. Keep the phone online and try again in ~30 seconds")
		}
		err = client.SendAppState(context.Background(), patch)
	}
	if err != nil {
		u.loggerWrapper.GetLogger(instance.Id).LogError("[%s] SaveContact %s: %v", instance.Id, jid.String(), err)
		return err
	}

	u.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Contact saved to addressbook: %s (%s)", instance.Id, fullName, jid.String())
	return nil
}
