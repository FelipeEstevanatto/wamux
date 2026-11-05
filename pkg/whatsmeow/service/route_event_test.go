package whatsmeow_service

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/felipeestevanatto/wamux/pkg/bgpool"
	"github.com/felipeestevanatto/wamux/pkg/config"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func mediaEvent() *events.Message {
	return &events.Message{Message: &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}}
}

func textEvent() *events.Message {
	return &events.Message{Message: &waE2E.Message{Conversation: proto.String("hi")}}
}

func routingClient(t *testing.T) *MyClient {
	t.Helper()
	logs := logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir()})
	t.Cleanup(func() { _ = logs.GetLogger("i1").Close() })
	return &MyClient{
		userID:        "i1",
		config:        &config.Config{WebhookFiles: true, DatabaseSaveMessages: true},
		mediaPool:     bgpool.New(2, 8),
		loggerWrapper: logs,
	}
}

// The reason routeEvent exists: whatsmeow handles an instance's events one at a
// time, so a download running inline holds up everything behind it.
func TestSlowMediaDoesNotBlockLaterEvents(t *testing.T) {
	mycli := routingClient(t)
	release := make(chan struct{})
	defer close(release)
	textDone := make(chan struct{})

	process := func(raw interface{}) {
		evt := raw.(*events.Message)
		if evt.Message.GetImageMessage() != nil {
			<-release // a download that takes minutes
			return
		}
		close(textDone)
	}

	// In a goroutine, so a regression fails the test instead of hanging it.
	go func() {
		mycli.routeEvent(mediaEvent(), process)
		mycli.routeEvent(textEvent(), process)
	}()

	select {
	case <-textDone:
	case <-time.After(2 * time.Second):
		t.Fatal("a text message waited behind a media download")
	}
}

// Events without a download stay inline, which is what keeps them ordered.
func TestEventsWithoutDownloadStayInline(t *testing.T) {
	mycli := routingClient(t)
	cases := map[string]interface{}{
		"text":    textEvent(),
		"receipt": &events.Receipt{},
	}
	for name, evt := range cases {
		ran := false
		mycli.routeEvent(evt, func(interface{}) { ran = true })
		if !ran {
			t.Fatalf("%s did not run inline", name)
		}
	}
}

// When nothing will be downloaded there is nothing slow to move off dispatch.
func TestMediaStaysInlineWhenDownloadIsDisabled(t *testing.T) {
	mycli := routingClient(t)
	mycli.config = &config.Config{WebhookFiles: false, DatabaseSaveMessages: true}

	ran := false
	mycli.routeEvent(mediaEvent(), func(interface{}) { ran = true })
	if !ran {
		t.Fatal("media with downloads disabled did not run inline")
	}
}

// whatsmeow recovers handler panics; a pool worker has no such net, so without
// this one bad message would crash every instance in the process.
func TestPanicInMediaWorkerIsContained(t *testing.T) {
	mycli := routingClient(t)
	mycli.routeEvent(mediaEvent(), func(interface{}) { panic("bad message") })

	// Both workers must survive repeated panics and still take work.
	for i := 0; i < 4; i++ {
		mycli.routeEvent(mediaEvent(), func(interface{}) { panic("bad message") })
	}
	done := make(chan struct{})
	mycli.routeEvent(mediaEvent(), func(interface{}) { close(done) })
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pool stopped taking work after a panic")
	}
}

func TestHasDownloadableMedia(t *testing.T) {
	child := func(m *waE2E.Message) *waE2E.Message {
		return &waE2E.Message{AssociatedChildMessage: &waE2E.FutureProofMessage{Message: m}}
	}
	cases := []struct {
		name string
		msg  *waE2E.Message
		want bool
	}{
		{"nil", nil, false},
		{"text", textEvent().Message, false},
		{"image", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}, true},
		{"audio", &waE2E.Message{AudioMessage: &waE2E.AudioMessage{}}, true},
		{"document", &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{}}, true},
		{"video", &waE2E.Message{VideoMessage: &waE2E.VideoMessage{}}, true},
		{"sticker", &waE2E.Message{StickerMessage: &waE2E.StickerMessage{}}, true},
		{"child image", child(&waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}), true},
		{"child text", child(textEvent().Message), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasDownloadableMedia(tc.msg); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func videoEvent(size uint64) *events.Message {
	return &events.Message{Message: &waE2E.Message{VideoMessage: &waE2E.VideoMessage{FileLength: &size}}}
}

// Large files are limited by memory, at about 20x their size at peak, so they run
// on their own small pool whatever the main pool allows.
func TestLargeMediaConcurrencyIsBounded(t *testing.T) {
	mycli := routingClient(t)
	mycli.mediaPool = bgpool.New(mediaWorkers, mediaQueueSize)
	mycli.largeMediaPool = bgpool.New(largeMediaWorkers, mediaQueueSize)

	release := make(chan struct{})
	var running, peak atomic.Int32
	var done sync.WaitGroup
	process := func(interface{}) {
		defer done.Done()
		peak.Store(max(peak.Load(), running.Add(1)))
		<-release
		running.Add(-1)
	}

	const files = 6
	done.Add(files)
	for i := 0; i < files; i++ {
		mycli.routeEvent(videoEvent(largeMediaBytes), process)
	}
	time.Sleep(100 * time.Millisecond)
	if got := running.Load(); got != largeMediaWorkers {
		t.Fatalf("%d large downloads running, want %d", got, largeMediaWorkers)
	}

	// Small media must not queue behind them.
	small := make(chan struct{})
	mycli.routeEvent(videoEvent(1024), func(interface{}) { close(small) })
	select {
	case <-small:
	case <-time.After(2 * time.Second):
		t.Fatal("small media waited behind large downloads")
	}

	close(release)
	done.Wait()
	if peak.Load() > largeMediaWorkers {
		t.Fatalf("peak %d, want at most %d", peak.Load(), largeMediaWorkers)
	}
}

func TestMediaFileLengthUsesLargestAttachment(t *testing.T) {
	big, small := uint64(9000), uint64(10)
	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{FileLength: &small},
		AssociatedChildMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{
			VideoMessage: &waE2E.VideoMessage{FileLength: &big},
		}},
	}
	if got := MediaFileLength(msg); got != big {
		t.Fatalf("got %d, want %d", got, big)
	}
	if got := MediaFileLength(nil); got != 0 {
		t.Fatalf("nil message: got %d", got)
	}
}
