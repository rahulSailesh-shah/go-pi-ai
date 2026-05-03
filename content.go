package gopiai

import (
	"encoding/json"
	"fmt"
)

type Content interface {
	Type() string
	isContent()
}

type TextContent struct {
	Text string `json:"text"`
}

func (t TextContent) Type() string { return "text" }
func (t TextContent) isContent()   {}

type ImageContent struct {
	URL      string `json:"url,omitempty"`
	Base64   string `json:"base64,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

func (i ImageContent) Type() string { return "image" }
func (i ImageContent) isContent()   {}

type ToolCall struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Arguments    map[string]any `json:"arguments"`
	RawArguments string         `json:"raw_arguments"`
}

func (t ToolCall) Type() string { return "toolCall" }
func (t ToolCall) isContent()   {}

func marshalContent(c Content) (json.RawMessage, error) {
	type contentWrapper struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	data, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	return json.Marshal(contentWrapper{Type: c.Type(), Data: data})
}

func unmarshalContent(raw json.RawMessage) (Content, error) {
	var wrapper struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, err
	}
	switch wrapper.Type {
	case "text":
		var c TextContent
		return c, json.Unmarshal(wrapper.Data, &c)
	case "image":
		var c ImageContent
		return c, json.Unmarshal(wrapper.Data, &c)
	case "toolCall":
		var c ToolCall
		return c, json.Unmarshal(wrapper.Data, &c)
	default:
		return nil, fmt.Errorf("unknown content type: %s", wrapper.Type)
	}
}

func marshalContents(contents []Content) ([]json.RawMessage, error) {
	result := make([]json.RawMessage, len(contents))
	for i, c := range contents {
		raw, err := marshalContent(c)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func unmarshalContents(raws []json.RawMessage) ([]Content, error) {
	result := make([]Content, len(raws))
	for i, raw := range raws {
		c, err := unmarshalContent(raw)
		if err != nil {
			return nil, err
		}
		result[i] = c
	}
	return result, nil
}
