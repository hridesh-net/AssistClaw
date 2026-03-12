package voice

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/assistclaw/assistclaw/internal/config"
)

type Client struct {
	cfg config.VoiceConfig
}

func NewClient(cfg config.VoiceConfig) *Client {
	if cfg.ServicePort == 0 {
		cfg.ServicePort = 11000
	}
	return &Client{cfg: cfg}
}

func (c *Client) STT(audioData []byte) (string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/stt", c.cfg.ServicePort)
	
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("audio", "audio.wav")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, bytes.NewReader(audioData)); err != nil {
		return "", err
	}
	writer.Close()

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("stt failed with status %d: %s", resp.StatusCode, string(b))
	}

	var res struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	return res.Text, nil
}

func (c *Client) TTS(text string, voiceRefPath string) ([]byte, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/tts", c.cfg.ServicePort)
	
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("text", text)
	
	// Option for reference audio (cloning)
	if voiceRefPath != "" {
		// Implement later if needed
	}
	
	writer.Close()

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tts failed with status %d: %s", resp.StatusCode, string(b))
	}

	return io.ReadAll(resp.Body)
}

func (c *Client) TTSDiscord(text string) ([][]byte, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/tts/discord", c.cfg.ServicePort)
	
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("text", text)
	writer.Close()

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tts-discord failed with status %d: %s", resp.StatusCode, string(b))
	}

	var res struct {
		Packets []string `json:"packets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	
	var out [][]byte
	for _, p := range res.Packets {
		dec, err := base64.StdEncoding.DecodeString(p)
		if err == nil {
			out = append(out, dec)
		}
	}
	return out, nil
}
