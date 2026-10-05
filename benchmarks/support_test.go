package benchmarks

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	instance_service "github.com/felipeestevanatto/wamux/pkg/instance/service"
	send_service "github.com/felipeestevanatto/wamux/pkg/sendMessage/service"
	"go.mau.fi/libsignal/groups"
	"go.mau.fi/libsignal/keys/identity"
	"go.mau.fi/libsignal/keys/prekey"
	"go.mau.fi/libsignal/protocol"
	"go.mau.fi/libsignal/serialize"
	"go.mau.fi/libsignal/session"
	"go.mau.fi/libsignal/state/record"
	sigstore "go.mau.fi/libsignal/state/store"
	"go.mau.fi/libsignal/tests"
	"go.mau.fi/libsignal/util/keyhelper"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// ---------------------------------------------------------------------------
// Signal group-send harness
//
// This mirrors exactly what whatsmew's Client.SendMessage does for a group
// (send.go sendGroup): one sender-key encryption of the body, then one pairwise
// Signal encryption of the small SenderKeyDistributionMessage per device.
// ---------------------------------------------------------------------------

var benchSer = serialize.NewProtoBufSerializer()

// benchRemote is the receiving side of a pairwise Signal session.
type benchRemote struct {
	address        *protocol.SignalAddress
	identity       *identity.KeyPair
	registrationID uint32
	deviceID       uint32
	preKeys        []*record.PreKey
	signedPreKey   *record.SignedPreKey
}

func newBenchRemote(name string, deviceID uint32) *benchRemote {
	ikp, _ := keyhelper.GenerateIdentityKeyPair()
	pks, _ := keyhelper.GeneratePreKeys(1, 1, benchSer.PreKeyRecord)
	spk, _ := keyhelper.GenerateSignedPreKey(ikp, 0, benchSer.SignedPreKeyRecord)
	return &benchRemote{
		address:        protocol.NewSignalAddress(name, deviceID),
		identity:       ikp,
		registrationID: keyhelper.GenerateRegistrationID(),
		deviceID:       deviceID,
		preKeys:        pks,
		signedPreKey:   spk,
	}
}

func (r *benchRemote) bundle() *prekey.Bundle {
	return prekey.NewBundle(
		r.registrationID, r.deviceID,
		r.preKeys[0].ID(), r.signedPreKey.ID(),
		r.preKeys[0].KeyPair().PublicKey(),
		r.signedPreKey.KeyPair().PublicKey(),
		r.signedPreKey.Signature(),
		r.identity.PublicKey(),
	)
}

// benchSender holds the stores whatsmeow keeps on Client.Store for one device.
type benchSender struct {
	sessionStore      *tests.InMemorySession
	preKeyStore       *tests.InMemoryPreKey
	signedPreKeyStore *tests.InMemorySignedPreKey
	identityStore     sigstore.IdentityKey
	senderKeyStore    *tests.InMemorySenderKey

	groupName    *protocol.SenderKeyName
	groupBuilder *groups.SessionBuilder
	groupCipher  *groups.GroupCipher
}

// newBenchSender builds a sender. Pass a custom identity store to model a
// database-backed identity store; nil uses the in-memory one.
func newBenchSender(identityStore sigstore.IdentityKey) *benchSender {
	if identityStore == nil {
		ikp, _ := keyhelper.GenerateIdentityKeyPair()
		identityStore = tests.NewInMemoryIdentityKey(ikp, keyhelper.GenerateRegistrationID())
	}
	s := &benchSender{
		sessionStore:      tests.NewInMemorySession(benchSer),
		preKeyStore:       tests.NewInMemoryPreKey(),
		signedPreKeyStore: tests.NewInMemorySignedPreKey(),
		identityStore:     identityStore,
		senderKeyStore:    tests.NewInMemorySenderKey(),
	}
	s.groupName = protocol.NewSenderKeyName("12345@g.us", protocol.NewSignalAddress("me", 1))
	s.groupBuilder = groups.NewGroupSessionBuilder(s.senderKeyStore, benchSer)
	if _, err := s.groupBuilder.Create(context.Background(), s.groupName); err != nil {
		panic(err)
	}
	s.groupCipher = groups.NewGroupCipher(s.groupBuilder, s.groupName, s.senderKeyStore)
	return s
}

// sessions establishes a pairwise session to every remote and returns one
// cipher per remote, matching GetUserDevices + WithCachedSessions + the
// per-device encrypt loop.
func (s *benchSender) sessions(remotes []*benchRemote) []*session.Cipher {
	ctx := context.Background()
	ciphers := make([]*session.Cipher, len(remotes))
	for i, r := range remotes {
		b := session.NewBuilder(s.sessionStore, s.preKeyStore, s.signedPreKeyStore, s.identityStore, r.address, benchSer)
		if err := b.ProcessBundle(ctx, r.bundle()); err != nil {
			panic(err)
		}
		ciphers[i] = session.NewCipher(b, r.address)
	}
	return ciphers
}

func benchRemotes(n int) []*benchRemote {
	out := make([]*benchRemote, n)
	for i := range out {
		out[i] = newBenchRemote("user"+itoa(i), 1)
	}
	return out
}

const benchMsgSize = 1024

func benchMsg() []byte {
	m := make([]byte, benchMsgSize)
	for i := range m {
		m[i] = byte(i)
	}
	return m
}

// latestSKDM renders the SKDM whatsmeow pairwise-encrypts to each device on
// every group send.
func latestSKDM(tb testing.TB, s *benchSender) []byte {
	skdm, err := s.groupBuilder.Create(context.Background(), s.groupName)
	if err != nil {
		tb.Fatal(err)
	}
	return skdm.Serialize()
}

// latencyIdentityStore wraps the in-memory identity store and models the one
// per-device database round trip whatsmeow still pays inside the group-send
// loop: SQLStore.IsTrustedIdentity issues a SELECT per device and is not batched
// (unlike sessions/LIDs, which c.getManySessions/the LID map batch). Set
// cache=true to model the in-memory identity cache that would remove it.
type latencyIdentityStore struct {
	*tests.InMemoryIdentityKey
	perCall time.Duration
	cache   bool

	mu    sync.RWMutex
	known map[string]bool
}

func (l *latencyIdentityStore) IsTrustedIdentity(ctx context.Context, addr *protocol.SignalAddress, key *identity.Key) (bool, error) {
	if l.cache {
		l.mu.RLock()
		v, ok := l.known[addr.String()]
		l.mu.RUnlock()
		if ok {
			return v, nil
		}
	}
	if l.perCall > 0 {
		time.Sleep(l.perCall)
	}
	v, err := l.InMemoryIdentityKey.IsTrustedIdentity(ctx, addr, key)
	if l.cache {
		l.mu.Lock()
		if l.known == nil {
			l.known = make(map[string]bool)
		}
		l.known[addr.String()] = v
		l.mu.Unlock()
	}
	return v, err
}

// ---------------------------------------------------------------------------
// HTTP harness fakes
// ---------------------------------------------------------------------------

// fakeInstanceService satisfies the auth middleware with an in-memory lookup,
// so the request pipeline can be benchmarked without Postgres.
type fakeInstanceService struct {
	instance_service.InstanceService
	instances map[string]*instance_model.Instance
}

var errUnknownToken = errors.New("not authorized")

func (f *fakeInstanceService) GetInstanceByToken(token string) (*instance_model.Instance, error) {
	if inst, ok := f.instances[token]; ok {
		return inst, nil
	}
	return nil, errUnknownToken
}

func (f *fakeInstanceService) GetAll() ([]*instance_model.Instance, error) {
	out := make([]*instance_model.Instance, 0, len(f.instances))
	for _, inst := range f.instances {
		out = append(out, inst)
	}
	return out, nil
}

// fakeSendService replaces the outbound WhatsApp call with a fixed realistic
// result. latency models the WhatsApp server round trip.
type fakeSendService struct {
	send_service.SendService
	latency time.Duration
}

func (f *fakeSendService) SendText(data *send_service.TextStruct, instance *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	if f.latency > 0 {
		time.Sleep(f.latency)
	}
	return sampleSendResult(data.Number, "text")
}

func (f *fakeSendService) SendMediaUrl(data *send_service.MediaStruct, instance *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	if f.latency > 0 {
		time.Sleep(f.latency)
	}
	return sampleSendResult(data.Number, "media")
}

func (f *fakeSendService) SendMediaFile(data *send_service.MediaStruct, fileData []byte, instance *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	return sampleSendResult(data.Number, "media")
}

func sampleSendResult(number, typ string) (*send_service.MessageSendStruct, error) {
	chat, err := types.ParseJID(number + "@s.whatsapp.net")
	if err != nil {
		return nil, err
	}
	return &send_service.MessageSendStruct{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     chat,
				Sender:   chat,
				IsFromMe: true,
			},
			ID:        "3EB0BENCH00000000001",
			Timestamp: time.Unix(1_760_000_000, 0),
			ServerID:  123456,
			Type:      typ,
		},
		Message: &waE2E.Message{Conversation: proto.String("benchmark")},
	}, nil
}

// itoa is a tiny local helper so the package does not pull strconv into the
// hot helpers for a single call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
