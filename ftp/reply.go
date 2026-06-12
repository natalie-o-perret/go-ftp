package ftp

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// ReplyScanner reads FTP replies from an io.Reader, transparently
// reassembling multi-line replies. The first line of every reply
// carries the 3-digit code that is used until the terminating line is
// observed.
type ReplyScanner struct {
	r   *bufio.Reader
	buf []string
}

// NewReplyScanner wraps r in a buffered reader.
func NewReplyScanner(r io.Reader) *ReplyScanner {
	return &ReplyScanner{r: bufio.NewReader(r)}
}

// Read returns the next complete reply.
func (s *ReplyScanner) Read() (Reply, error) {
	if s.r == nil {
		return Reply{}, io.EOF
	}
	if len(s.buf) > 0 {
		first := s.buf[0]
		s.buf = s.buf[1:]
		firstCode, firstText, ok := splitReplyLine(first)
		if !ok {
			return Reply{}, fmt.Errorf("ftp: malformed reply %q", first)
		}
		reply := Reply{Code: firstCode, Lines: []string{firstText}}
		if strings.HasSuffix(first, "\r\n") == false {
			reply.Lines = reply.Lines[:0]
		}
		if isMultiLineStart(first) {
			for {
				line, err := s.r.ReadString('\n')
				if err != nil {
					return Reply{}, err
				}
				code, text, ok := splitReplyLine(line)
				if !ok {
					return Reply{}, fmt.Errorf("ftp: malformed continuation %q", line)
				}
				if code != firstCode {
					return Reply{}, fmt.Errorf("ftp: reply code changed mid-message: %d then %d", firstCode, code)
				}
				if !isMultiLineStart(line) {
					reply.Lines = append(reply.Lines, text)
					return reply, nil
				}
				reply.Lines = append(reply.Lines, text)
			}
		}
		return reply, nil
	}

	first, err := s.r.ReadString('\n')
	if err != nil {
		return Reply{}, err
	}
	firstCode, firstText, ok := splitReplyLine(first)
	if !ok {
		return Reply{}, fmt.Errorf("ftp: malformed reply %q", first)
	}
	reply := Reply{Code: firstCode, Lines: []string{firstText}}
	if isMultiLineStart(first) {
		for {
			line, err := s.r.ReadString('\n')
			if err != nil {
				return Reply{}, err
			}
			code, text, ok := splitReplyLine(line)
			if !ok {
				return Reply{}, fmt.Errorf("ftp: malformed continuation %q", line)
			}
			if code != firstCode {
				return Reply{}, fmt.Errorf("ftp: reply code changed mid-message: %d then %d", firstCode, code)
			}
			reply.Lines = append(reply.Lines, text)
			if !isMultiLineStart(line) {
				return reply, nil
			}
		}
	}
	return reply, nil
}

func splitReplyLine(line string) (Code, string, bool) {
	if len(line) < 3 {
		return 0, "", false
	}
	code, err := parseCode(line[:3])
	if err != nil {
		return 0, "", false
	}
	rest := line[3:]
	switch {
	case strings.HasPrefix(rest, " "):
		return code, strings.TrimRight(rest[1:], "\r\n"), true
	case strings.HasPrefix(rest, "-"):
		return code, strings.TrimRight(rest[1:], "\r\n"), true
	default:
		return 0, "", false
	}
}

func parseCode(s string) (Code, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("ftp: non-digit in reply code %q", s)
		}
		n = n*10 + int(c-'0')
	}
	return Code(n), nil
}

func isMultiLineStart(line string) bool {
	return len(line) >= 4 && line[3] == '-'
}
