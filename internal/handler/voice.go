package handler

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"movieagent/internal/service/voice"
	"movieagent/pkg/response"

	"github.com/gin-gonic/gin"
)

const (
	voiceSessionCookie = "voice_session"
	maxVoiceBytes      = 8 << 20
)

type VoiceHandler struct {
	STT *voice.Service
}

func NewVoiceHandler(stt *voice.Service) *VoiceHandler {
	return &VoiceHandler{STT: stt}
}

func (h *VoiceHandler) Status(c *gin.Context) {
	if h.STT == nil {
		response.Fail(c, http.StatusServiceUnavailable, 1, "voice stt is not initialized")
		return
	}
	err := h.STT.Ready()
	ready := err == nil
	msg := "ok"
	if err != nil {
		msg = err.Error()
	}
	response.OK(c, gin.H{
		"ready":   ready,
		"engine":  "whisper-local",
		"model":   h.STT.Model(),
		"python":  h.STT.Python(),
		"message": msg,
	})
}

func (h *VoiceHandler) Transcribe(c *gin.Context) {
	if h.STT == nil {
		response.Fail(c, http.StatusServiceUnavailable, 1, "voice stt is not initialized")
		return
	}
	if err := h.STT.Ready(); err != nil {
		response.Fail(c, http.StatusServiceUnavailable, 1, err.Error())
		return
	}

	sessionID := h.ensureSession(c)
	started := time.Now()

	data, ext, err := readVoiceUpload(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, 1, err.Error())
		return
	}
	tmp, err := os.CreateTemp("", "movieagent-voice-*"+ext)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, 1, err.Error())
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		response.Fail(c, http.StatusInternalServerError, 1, err.Error())
		return
	}
	if err := tmp.Close(); err != nil {
		response.Fail(c, http.StatusInternalServerError, 1, err.Error())
		return
	}

	text, err := h.STT.TranscribeFile(tmpPath)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, 1, err.Error())
		return
	}
	ms := time.Since(started).Milliseconds()
	var line voice.Line
	if text != "" {
		line = h.STT.Append(sessionID, text, ms)
	}
	response.OK(c, gin.H{
		"text":       text,
		"empty":      text == "",
		"ms":         ms,
		"utterance":  line,
		"session_id": sessionID,
		"collected":  h.STT.List(sessionID),
	})
}

func (h *VoiceHandler) ListTranscripts(c *gin.Context) {
	sessionID := h.ensureSession(c)
	response.OK(c, gin.H{
		"session_id": sessionID,
		"collected":  h.STT.List(sessionID),
	})
}

func (h *VoiceHandler) ClearTranscripts(c *gin.Context) {
	sessionID := h.ensureSession(c)
	h.STT.Clear(sessionID)
	response.OK(c, gin.H{"session_id": sessionID, "collected": []voice.Line{}})
}

func (h *VoiceHandler) ensureSession(c *gin.Context) string {
	if id := strings.TrimSpace(c.GetHeader("X-Voice-Session")); id != "" {
		return id
	}
	if cookie, err := c.Cookie(voiceSessionCookie); err == nil && strings.TrimSpace(cookie) != "" {
		return cookie
	}
	id := newSessionID()
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(voiceSessionCookie, id, 7*24*3600, "/", "", false, false)
	return id
}

func newSessionID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().Format("sess-150405.000")
	}
	return "sess-" + hex.EncodeToString(b[:])
}

func readVoiceUpload(c *gin.Context) ([]byte, string, error) {
	if fh, err := c.FormFile("file"); err == nil && fh != nil {
		f, err := fh.Open()
		if err != nil {
			return nil, "", err
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, maxVoiceBytes+1))
		if err != nil {
			return nil, "", err
		}
		if int64(len(data)) > maxVoiceBytes {
			return nil, "", errVoiceTooLarge
		}
		return data, extFromName(fh.Filename, fh.Header.Get("Content-Type")), nil
	}
	data, err := io.ReadAll(io.LimitReader(c.Request.Body, maxVoiceBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 {
		return nil, "", errVoiceMissing
	}
	if int64(len(data)) > maxVoiceBytes {
		return nil, "", errVoiceTooLarge
	}
	return data, extFromName("", c.ContentType()), nil
}

var (
	errVoiceMissing  = errString("missing audio")
	errVoiceTooLarge = errString("audio too large")
)

type errString string

func (e errString) Error() string { return string(e) }

func extFromName(name, mime string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".wav", ".webm", ".ogg", ".mp3", ".m4a", ".flac":
		return ext
	}
	mime = strings.ToLower(mime)
	switch {
	case strings.Contains(mime, "wav"):
		return ".wav"
	case strings.Contains(mime, "webm"):
		return ".webm"
	case strings.Contains(mime, "ogg"):
		return ".ogg"
	case strings.Contains(mime, "mpeg"):
		return ".mp3"
	default:
		return ".wav"
	}
}
