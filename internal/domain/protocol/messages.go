package protocol

import (
	"encoding/json"
	"fmt"
	"math"

	"ghostwire/internal/domain"
)

type HelloPayload struct {
	ProtocolVersion uint8  `json:"protocolVersion"`
	ClientName      string `json:"clientName"`
	Token           string `json:"token"`
}

type WelcomePayload struct {
	Ok            bool        `json:"ok"`
	Reason        string      `json:"reason,omitempty"`
	ServerVersion string      `json:"serverVersion,omitempty"`
	Screen        *ScreenSize `json:"screen,omitempty"`
	Fps           *int        `json:"fps,omitempty"`
}

type ScreenSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type PingPayload struct {
	T int64 `json:"t"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ByePayload struct {
	Reason string `json:"reason"`
}

type ScreenInfoPayload struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

func parseObject(payload []byte) (map[string]any, error) {
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return nil, domain.WrapError(err, domain.ErrProtocol, "invalid JSON payload")
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, domain.NewError(domain.ErrProtocol, "payload must be a JSON object")
	}
	return obj, nil
}

func requireNumber(obj map[string]any, key string) (float64, error) {
	v, ok := obj[key]
	if !ok {
		return 0, domain.NewError(domain.ErrProtocol, fmt.Sprintf("missing or invalid field %q", key))
	}
	f, ok := v.(float64)
	if !ok || math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, domain.NewError(domain.ErrProtocol, fmt.Sprintf("missing or invalid field %q", key))
	}
	return f, nil
}

func requireString(obj map[string]any, key string, maxLen int) (string, error) {
	v, ok := obj[key]
	if !ok {
		return "", domain.NewError(domain.ErrProtocol, fmt.Sprintf("missing or invalid field %q", key))
	}
	s, ok := v.(string)
	if !ok {
		return "", domain.NewError(domain.ErrProtocol, fmt.Sprintf("missing or invalid field %q", key))
	}
	if len(s) > maxLen {
		return "", domain.NewError(domain.ErrProtocol,
			fmt.Sprintf("field %q exceeds %d characters", key, maxLen))
	}
	return s, nil
}

func optionalString(obj map[string]any, key string, maxLen int) (string, error) {
	v, ok := obj[key]
	if v == nil || !ok {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", domain.NewError(domain.ErrProtocol, fmt.Sprintf("invalid field %q", key))
	}
	if len(s) > maxLen {
		return "", domain.NewError(domain.ErrProtocol,
			fmt.Sprintf("field %q exceeds %d characters", key, maxLen))
	}
	return s, nil
}

func optionalInt(obj map[string]any, key string) (*int, error) {
	v, ok := obj[key]
	if !ok || v == nil {
		return nil, nil
	}
	f, ok := v.(float64)
	if !ok {
		return nil, domain.NewError(domain.ErrProtocol, fmt.Sprintf("invalid field %q", key))
	}
	n := int(f)
	return &n, nil
}

func parseScreen(obj map[string]any) (*ScreenSize, error) {
	screen, ok := obj["screen"]
	if !ok {
		return nil, domain.NewError(domain.ErrProtocol, "missing or invalid field \"screen\"")
	}
	s, ok := screen.(map[string]any)
	if !ok {
		return nil, domain.NewError(domain.ErrProtocol, "missing or invalid field \"screen\"")
	}
	wf, err := requireNumber(s, "width")
	if err != nil {
		return nil, err
	}
	hf, err := requireNumber(s, "height")
	if err != nil {
		return nil, err
	}
	w, h := int(wf), int(hf)
	if w <= 0 || h <= 0 || w > 65535 || h > 65535 {
		return nil, domain.NewError(domain.ErrProtocol, "screen dimensions out of range")
	}
	return &ScreenSize{Width: w, Height: h}, nil
}

func EncodeHello(payload HelloPayload) []byte {
	b, _ := json.Marshal(map[string]any{
		"protocolVersion": payload.ProtocolVersion,
		"clientName":      payload.ClientName,
		"token":           payload.Token,
	})
	return b
}

func DecodeHello(payload []byte) (HelloPayload, error) {
	obj, err := parseObject(payload)
	if err != nil {
		return HelloPayload{}, err
	}
	pv, err := requireNumber(obj, "protocolVersion")
	if err != nil {
		return HelloPayload{}, err
	}
	if pv != math.Trunc(pv) || pv < 0 || pv > 255 {
		return HelloPayload{}, domain.NewError(domain.ErrProtocol, "invalid protocolVersion")
	}
	cn, err := requireString(obj, "clientName", 64)
	if err != nil {
		return HelloPayload{}, err
	}
	tk, err := requireString(obj, "token", 512)
	if err != nil {
		return HelloPayload{}, err
	}
	return HelloPayload{ProtocolVersion: uint8(pv), ClientName: cn, Token: tk}, nil
}

func EncodeWelcome(payload WelcomePayload) []byte {
	out := map[string]any{"ok": payload.Ok}
	if payload.Reason != "" {
		out["reason"] = payload.Reason
	}
	if payload.ServerVersion != "" {
		out["serverVersion"] = payload.ServerVersion
	}
	if payload.Screen != nil {
		out["screen"] = payload.Screen
	}
	if payload.Fps != nil {
		out["fps"] = *payload.Fps
	}
	b, _ := json.Marshal(out)
	return b
}

func DecodeWelcome(payload []byte) (WelcomePayload, error) {
	obj, err := parseObject(payload)
	if err != nil {
		return WelcomePayload{}, err
	}
	okVal, ok := obj["ok"].(bool)
	if !ok {
		return WelcomePayload{}, domain.NewError(domain.ErrProtocol, "missing or invalid field \"ok\"")
	}
	reason, err := optionalString(obj, "reason", 256)
	if err != nil {
		return WelcomePayload{}, err
	}
	if !okVal {
		return WelcomePayload{Ok: false, Reason: reason}, nil
	}
	sv, err := optionalString(obj, "serverVersion", 32)
	if err != nil {
		return WelcomePayload{}, err
	}
	fps, err := optionalInt(obj, "fps")
	if err != nil {
		return WelcomePayload{}, err
	}
	screen, err := parseScreen(obj)
	if err != nil {
		return WelcomePayload{}, err
	}
	return WelcomePayload{
		Ok:            true,
		ServerVersion: sv,
		Screen:        screen,
		Fps:           fps,
	}, nil
}

func EncodePing(payload PingPayload) []byte {
	b, _ := json.Marshal(map[string]any{"t": payload.T})
	return b
}

func DecodePing(payload []byte) (PingPayload, error) {
	obj, err := parseObject(payload)
	if err != nil {
		return PingPayload{}, err
	}
	t, err := requireNumber(obj, "t")
	if err != nil {
		return PingPayload{}, err
	}
	return PingPayload{T: int64(t)}, nil
}

func EncodeError(payload ErrorPayload) []byte {
	b, _ := json.Marshal(map[string]any{"code": payload.Code, "message": payload.Message})
	return b
}

func DecodeError(payload []byte) (ErrorPayload, error) {
	obj, err := parseObject(payload)
	if err != nil {
		return ErrorPayload{}, err
	}
	code, err := requireString(obj, "code", 64)
	if err != nil {
		return ErrorPayload{}, err
	}
	msg, err := requireString(obj, "message", 512)
	if err != nil {
		return ErrorPayload{}, err
	}
	return ErrorPayload{Code: code, Message: msg}, nil
}

func EncodeBye(payload ByePayload) []byte {
	b, _ := json.Marshal(map[string]any{"reason": payload.Reason})
	return b
}

func DecodeBye(payload []byte) (ByePayload, error) {
	obj, err := parseObject(payload)
	if err != nil {
		return ByePayload{}, err
	}
	r, err := requireString(obj, "reason", 256)
	if err != nil {
		return ByePayload{}, err
	}
	return ByePayload{Reason: r}, nil
}

func EncodeScreenInfo(payload ScreenInfoPayload) []byte {
	b, _ := json.Marshal(map[string]any{"width": payload.Width, "height": payload.Height})
	return b
}

func DecodeScreenInfo(payload []byte) (ScreenInfoPayload, error) {
	obj, err := parseObject(payload)
	if err != nil {
		return ScreenInfoPayload{}, err
	}
	wf, err := requireNumber(obj, "width")
	if err != nil {
		return ScreenInfoPayload{}, err
	}
	hf, err := requireNumber(obj, "height")
	if err != nil {
		return ScreenInfoPayload{}, err
	}
	w, h := int(wf), int(hf)
	if w <= 0 || h <= 0 || w > 65535 || h > 65535 {
		return ScreenInfoPayload{}, domain.NewError(domain.ErrProtocol, "screen dimensions out of range")
	}
	return ScreenInfoPayload{Width: w, Height: h}, nil
}
