package server_handler

import (
	"bufio"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	message_repository "github.com/evolution-foundation/evolution-go/pkg/message/repository"
	whatsmeow_service "github.com/evolution-foundation/evolution-go/pkg/whatsmeow/service"
	"github.com/gin-gonic/gin"
)

type ServerHandler interface {
	ServerOk(ctx *gin.Context)
	Stats(ctx *gin.Context)
	InstanceOverview(ctx *gin.Context)
}

// OverviewProvider is the slice of the whatsmeow service the dashboard needs to
// show each instance's own profile picture and contact count, and to turn the
// aggregated message sources into display names.
type OverviewProvider interface {
	GetInstanceOverview(instanceId string) (*whatsmeow_service.InstanceOverview, error)
	ResolveChats(users []string) map[string]whatsmeow_service.ChatIdentity
}

type serverHandler struct {
	messageRepo message_repository.MessageRepository
	overview    OverviewProvider
	version     string
	startTime   time.Time
}

// ServerOk implements ServerHandler.
// @Summary Server health
// @Description Returns ok when the server is up (public, no apikey)
// @Tags Server
// @Produce json
// @Success 200 {object} gin.H "status"
// @Router /server/ok [get]
func (s *serverHandler) ServerOk(ctx *gin.Context) {
	ctx.JSON(200, gin.H{
		"status": "ok",
	})
}

// Stats returns system metrics (Go runtime + Linux host) and message stats.
// Used by the self-hosted dashboard (GET /dashboard). Auth: AuthAdmin.
// @Summary System and message metrics
// @Description Runtime/host metrics (version, RAM, load, goroutines, uptime) and message aggregates
// @Tags Server
// @Produce json
// @Success 200 {object} gin.H "system and messages"
// @Router /server/stats [get]
func (s *serverHandler) Stats(ctx *gin.Context) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	const mb = 1024.0 * 1024.0
	system := gin.H{
		"version":       s.version,
		"goroutines":    runtime.NumGoroutine(),
		"numCpu":        runtime.NumCPU(),
		"goVersion":     runtime.Version(),
		"uptimeSeconds": int64(time.Since(s.startTime).Seconds()),
		"memAllocMB":    float64(mem.Alloc) / mb,
		"memSysMB":      float64(mem.Sys) / mb,
		"heapInuseMB":   float64(mem.HeapInuse) / mb,
		"numGC":         mem.NumGC,
	}

	if l1, l5, l15, ok := readLoadAvg(); ok {
		system["loadAvg1"] = l1
		system["loadAvg5"] = l5
		system["loadAvg15"] = l15
	}
	if totalKB, availKB, ok := readHostMem(); ok {
		system["hostMemTotalMB"] = totalKB / 1024.0
		system["hostMemAvailableMB"] = availKB / 1024.0
		if totalKB > 0 {
			system["hostMemUsedPct"] = (1 - availKB/totalKB) * 100
		}
	}

	messages := gin.H{"total": 0}
	if s.messageRepo != nil {
		if st, err := s.messageRepo.GetStats(); err == nil {
			messages = gin.H{
				"total":      st.Total,
				"byStatus":   st.ByStatus,
				"byDay":      st.ByDay,
				"topSources": s.resolveTopSources(st.TopSources),
			}
		}
	}

	ctx.JSON(200, gin.H{"system": system, "messages": messages})
}

// sourceEntry is a top source annotated with a resolved display name.
type sourceEntry struct {
	Key   string `json:"key"`
	Name  string `json:"name,omitempty"`
	Phone string `json:"phone,omitempty"`
	Count int64  `json:"count"`
}

// topSourcesLimit is how many conversations the dashboard shows once LID/phone
// duplicates have been merged.
const topSourcesLimit = 8

// resolveTopSources annotates the aggregated sources with a display name and
// merges rows that are the same conversation. A contact persisted once under a
// LID and once under its phone number must not show up twice: both resolve to the
// same phone, which becomes the canonical key. Names come from the whatsmeow
// contact/LID/group stores and are best-effort — an unresolved source keeps its
// raw key and the frontend falls back to "+<key>".
func (s *serverHandler) resolveTopSources(sources []message_repository.StatKV) []sourceEntry {
	out := make([]sourceEntry, 0, len(sources))
	if len(sources) == 0 {
		return out
	}
	users := make([]string, 0, len(sources))
	for _, kv := range sources {
		users = append(users, kv.Key)
	}
	var identities map[string]whatsmeow_service.ChatIdentity
	if s.overview != nil {
		identities = s.overview.ResolveChats(users)
	}

	index := make(map[string]int, len(sources))
	for _, kv := range sources {
		id := identities[kv.Key]
		canonical := kv.Key
		if id.Phone != "" {
			canonical = id.Phone
		}
		if pos, ok := index[canonical]; ok {
			out[pos].Count += kv.Count
			if out[pos].Name == "" {
				out[pos].Name = id.Name
			}
			if out[pos].Phone == "" {
				out[pos].Phone = id.Phone
			}
			continue
		}
		index[canonical] = len(out)
		out = append(out, sourceEntry{Key: canonical, Name: id.Name, Phone: id.Phone, Count: kv.Count})
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	if len(out) > topSourcesLimit {
		out = out[:topSourcesLimit]
	}
	return out
}

// InstanceOverview returns the per-instance dashboard summary: the account's own
// profile picture, push name and local contact count. Auth: AuthAdmin.
// @Summary Per-instance overview
// @Description Own profile picture, push name, local contact count and persisted message count
// @Tags Instance
// @Produce json
// @Param instanceId path string true "Instance Id"
// @Success 200 {object} gin.H "overview"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /instance/overview/{instanceId} [get]
func (s *serverHandler) InstanceOverview(ctx *gin.Context) {
	instanceId := ctx.Param("instanceId")
	if instanceId == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "instanceId is required"})
		return
	}
	if s.overview == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "instance overview unavailable"})
		return
	}
	overview, err := s.overview.GetInstanceOverview(instanceId)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// The message/chat counts come from the local messages table, not whatsmeow.
	if s.messageRepo != nil {
		if total, merr := s.messageRepo.CountByInstance(instanceId); merr == nil {
			overview.MessagesCount = total
		}
		if chats, cerr := s.messageRepo.CountChatsByInstance(instanceId); cerr == nil {
			overview.ChatsCount = chats
		}
	}
	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": overview})
}

// readLoadAvg reads /proc/loadavg (Linux). Returns the 1/5/15 min averages.
func readLoadAvg() (float64, float64, float64, bool) {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0, false
	}
	f := strings.Fields(string(b))
	if len(f) < 3 {
		return 0, 0, 0, false
	}
	l1, _ := strconv.ParseFloat(f[0], 64)
	l5, _ := strconv.ParseFloat(f[1], 64)
	l15, _ := strconv.ParseFloat(f[2], 64)
	return l1, l5, l15, true
}

// readHostMem reads /proc/meminfo (Linux). Returns MemTotal and MemAvailable in kB.
func readHostMem() (float64, float64, bool) {
	fp, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, false
	}
	defer fp.Close()

	var total, avail float64
	sc := bufio.NewScanner(fp)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			total = parseMeminfoKB(line)
		} else if strings.HasPrefix(line, "MemAvailable:") {
			avail = parseMeminfoKB(line)
		}
	}
	if total == 0 {
		return 0, 0, false
	}
	return total, avail, true
}

func parseMeminfoKB(line string) float64 {
	f := strings.Fields(line)
	if len(f) < 2 {
		return 0
	}
	v, _ := strconv.ParseFloat(f[1], 64)
	return v
}

func NewServerHandler(messageRepo message_repository.MessageRepository, version string, overview OverviewProvider) ServerHandler {
	return &serverHandler{messageRepo: messageRepo, overview: overview, version: version, startTime: time.Now()}
}
