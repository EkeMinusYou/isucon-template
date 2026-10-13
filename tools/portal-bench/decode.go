package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

func extract(body []byte, r response) (map[string]string, error) {
	var document any
	if r.Format == "json" {
		d := json.NewDecoder(bytes.NewReader(body))
		d.UseNumber()
		if err := d.Decode(&document); err != nil {
			return nil, fmt.Errorf("invalid JSON response")
		}
		var extra any
		if err := d.Decode(&extra); err != io.EOF {
			return nil, fmt.Errorf("trailing JSON data")
		}
	}
	fields := make(map[string]string)
	for name, s := range r.Fields {
		var value string
		var found bool
		var err error
		switch r.Format {
		case "json":
			var v any
			v, found, err = jsonPointer(document, s.Path)
			if err == nil && found {
				value, err = scalarString(v)
			}
		case "protobuf":
			value, found, err = protoSelect(body, s)
		case "text":
			value, found = string(body), true
		case "regex":
			r, compileErr := regexp.Compile(s.Path)
			if compileErr != nil {
				return nil, compileErr
			}
			m := r.FindSubmatch(body)
			if len(m) == 2 {
				value, found = string(m[1]), true
			}
		default:
			return nil, fmt.Errorf("unsupported response format")
		}
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", name, err)
		}
		if !found {
			if s.Default == nil {
				return nil, fmt.Errorf("required field %s is missing", name)
			}
			value = *s.Default
		}
		if s.Type == "html_text" {
			value = regexp.MustCompile(`(?i)<br\s*/?>`).ReplaceAllString(value, "\n")
			value = regexp.MustCompile(`(?s)<[^>]*>`).ReplaceAllString(value, "")
			value = html.UnescapeString(value)
		}
		for _, replacement := range s.Replace {
			r, err := regexp.Compile(replacement.Pattern)
			if err != nil {
				return nil, err
			}
			value = r.ReplaceAllString(value, replacement.Value)
		}
		if s.Trim {
			value = strings.TrimSpace(value)
		}
		fields[name] = value
	}
	return fields, nil
}

func jsonPointer(root any, path string) (any, bool, error) {
	if path == "" {
		return root, root != nil, nil
	}
	if !strings.HasPrefix(path, "/") {
		return nil, false, fmt.Errorf("invalid JSON pointer")
	}
	value := root
	for _, token := range strings.Split(path[1:], "/") {
		token = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
		switch container := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = container[token]
			if !ok || value == nil {
				return nil, false, nil
			}
		case []any:
			i, err := strconv.Atoi(token)
			if err != nil || i < 0 || i >= len(container) {
				return nil, false, nil
			}
			value = container[i]
		default:
			return nil, false, nil
		}
	}
	return value, value != nil, nil
}

type protoStep struct{ number, index int }

func parseProtoPath(path string) ([]protoStep, error) {
	var steps []protoStep
	for _, part := range strings.Split(path, ".") {
		m := regexp.MustCompile(`^([1-9][0-9]*)(?:\[([0-9]+)\])?$`).FindStringSubmatch(part)
		if m == nil {
			return nil, fmt.Errorf("invalid protobuf path %q", path)
		}
		n, err := strconv.Atoi(m[1])
		if err != nil || n >= 1<<29 {
			return nil, fmt.Errorf("invalid protobuf field number")
		}
		i := 0
		if m[2] != "" {
			i, err = strconv.Atoi(m[2])
			if err != nil {
				return nil, err
			}
		}
		steps = append(steps, protoStep{n, i})
	}
	return steps, nil
}

type protoValue struct {
	wire   int
	number uint64
	data   []byte
}

func protoFields(body []byte) (map[int][]protoValue, error) {
	fields := make(map[int][]protoValue)
	for len(body) > 0 {
		tag, n := binary.Uvarint(body)
		if n <= 0 || tag>>3 == 0 || tag>>3 >= 1<<29 {
			return nil, fmt.Errorf("invalid protobuf tag")
		}
		body = body[n:]
		v := protoValue{wire: int(tag & 7)}
		switch v.wire {
		case 0:
			v.number, n = binary.Uvarint(body)
			if n <= 0 {
				return nil, fmt.Errorf("invalid protobuf varint")
			}
			body = body[n:]
		case 1:
			if len(body) < 8 {
				return nil, fmt.Errorf("truncated fixed64")
			}
			v.number = binary.LittleEndian.Uint64(body)
			body = body[8:]
		case 2:
			length, n := binary.Uvarint(body)
			if n <= 0 || length > uint64(len(body)-n) {
				return nil, fmt.Errorf("truncated protobuf bytes")
			}
			v.data = body[n : n+int(length)]
			body = body[n+int(length):]
		case 5:
			if len(body) < 4 {
				return nil, fmt.Errorf("truncated fixed32")
			}
			v.number = uint64(binary.LittleEndian.Uint32(body))
			body = body[4:]
		default:
			return nil, fmt.Errorf("unsupported protobuf wire type %d", v.wire)
		}
		fields[int(tag>>3)] = append(fields[int(tag>>3)], v)
	}
	return fields, nil
}

func protoSelect(body []byte, s selector) (string, bool, error) {
	steps, err := parseProtoPath(s.Path)
	if err != nil {
		return "", false, err
	}
	var value protoValue
	for i, step := range steps {
		fields, err := protoFields(body)
		if err != nil {
			return "", false, err
		}
		values := fields[step.number]
		if step.index >= len(values) {
			return "", false, nil
		}
		value = values[step.index]
		if i+1 < len(steps) {
			if value.wire != 2 {
				return "", false, fmt.Errorf("nested field is not a message")
			}
			body = value.data
		}
	}
	typ := s.Type
	if typ == "" || typ == "auto" {
		if value.wire == 2 {
			typ = "string"
		} else {
			typ = "uint"
		}
	}
	if value.wire == 2 {
		switch typ {
		case "string":
			if !utf8.Valid(value.data) {
				return "", false, fmt.Errorf("invalid UTF-8 string")
			}
			return string(value.data), true, nil
		case "bytes":
			return base64.StdEncoding.EncodeToString(value.data), true, nil
		}
	} else {
		switch typ {
		case "uint":
			return strconv.FormatUint(value.number, 10), true, nil
		case "int":
			return strconv.FormatInt(int64(value.number), 10), true, nil
		case "sint":
			return strconv.FormatInt(int64(value.number>>1)^-int64(value.number&1), 10), true, nil
		case "bool":
			return strconv.FormatBool(value.number != 0), true, nil
		case "float32":
			if value.wire == 5 {
				return strconv.FormatFloat(float64(math.Float32frombits(uint32(value.number))), 'g', -1, 32), true, nil
			}
		case "float64":
			if value.wire == 1 {
				return strconv.FormatFloat(math.Float64frombits(value.number), 'g', -1, 64), true, nil
			}
		}
	}
	return "", false, fmt.Errorf("type %s does not match wire type %d", typ, value.wire)
}
