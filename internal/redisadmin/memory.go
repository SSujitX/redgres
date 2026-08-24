package redisadmin

import (
	"context"
	"strings"
)

type ACLSetUserCall struct {
	Username string
	Rules    []string
}

type MemoryClient struct {
	PingErr         error
	InfoErr         error
	InfoText        string
	DBSizeErr       error
	Size            int64
	ACLListErr      error
	ACLLines        []string
	ACLSetUserErr   error
	ACLSetUserCalls []ACLSetUserCall
}

func (m *MemoryClient) Ping(context.Context) error {
	if m.PingErr != nil {
		return m.PingErr
	}
	return nil
}

func (m *MemoryClient) Info(context.Context) (string, error) {
	if m.InfoErr != nil {
		return "", m.InfoErr
	}
	return m.InfoText, nil
}

func (m *MemoryClient) DBSize(context.Context) (int64, error) {
	if m.DBSizeErr != nil {
		return 0, m.DBSizeErr
	}
	return m.Size, nil
}

func (m *MemoryClient) ACLList(context.Context) ([]string, error) {
	if m.ACLListErr != nil {
		return nil, m.ACLListErr
	}
	if m.ACLLines == nil {
		return []string{}, nil
	}
	return m.ACLLines, nil
}

func (m *MemoryClient) ACLSetUser(_ context.Context, username string, rules ...string) error {
	if m.ACLSetUserErr != nil {
		return m.ACLSetUserErr
	}
	copied := append([]string(nil), rules...)
	m.ACLSetUserCalls = append(m.ACLSetUserCalls, ACLSetUserCall{Username: username, Rules: copied})
	line := "user " + username
	if len(copied) > 0 {
		line += " " + strings.Join(copied, " ")
	}
	replaced := false
	for i, existing := range m.ACLLines {
		fields := strings.Fields(existing)
		if len(fields) >= 2 && fields[0] == "user" && fields[1] == username {
			m.ACLLines[i] = line
			replaced = true
			break
		}
	}
	if !replaced {
		m.ACLLines = append(m.ACLLines, line)
	}
	return nil
}
