package email

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/knadh/smtppool/v2"
)

type smtpTestRequest struct {
	Name             string              `json:"name"`
	UUID             string              `json:"uuid"`
	Enabled          bool                `json:"enabled"`
	Host             string              `json:"host"`
	HelloHostname    string              `json:"hello_hostname"`
	Port             int                 `json:"port"`
	AuthProtocol     string              `json:"auth_protocol"`
	Username         string              `json:"username"`
	Password         string              `json:"password"`
	EmailHeaders     []map[string]string `json:"email_headers"`
	FromAddresses    []string            `json:"from_addresses"`
	DefaultFromEmail string              `json:"default_from_email"`
	MaxConns         int                 `json:"max_conns"`
	MaxMsgRetries    int                 `json:"max_msg_retries"`
	IdleTimeout      string              `json:"idle_timeout"`
	WaitTimeout      string              `json:"wait_timeout"`
	TLSType          string              `json:"tls_type"`
	TLSSkipVerify    bool                `json:"tls_skip_verify"`
	Email            string              `json:"email"`
}

// ParseSMTPTestRequest converts the API request payload into an SMTP server config
// and the destination test address.
func ParseSMTPTestRequest(body []byte) (Server, string, string, error) {
	var req smtpTestRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return Server{}, "", "", err
	}

	idleTimeout, err := time.ParseDuration(req.IdleTimeout)
	if err != nil {
		return Server{}, "", "", err
	}

	waitTimeout, err := time.ParseDuration(req.WaitTimeout)
	if err != nil {
		return Server{}, "", "", err
	}

	return Server{
		Name:          req.Name,
		Username:      req.Username,
		Password:      req.Password,
		AuthProtocol:  req.AuthProtocol,
		TLSType:       req.TLSType,
		TLSSkipVerify: req.TLSSkipVerify,
		EmailHeaders:  flattenEmailHeaders(req.EmailHeaders),
		Opt: smtppool.Opt{
			Host:              req.Host,
			Port:              req.Port,
			HelloHostname:     req.HelloHostname,
			MaxConns:          req.MaxConns,
			MaxMessageRetries: req.MaxMsgRetries,
			IdleTimeout:       idleTimeout,
			PoolWaitTimeout:   waitTimeout,
		},
	}, strings.TrimSpace(req.Email), strings.TrimSpace(req.DefaultFromEmail), nil
}

func flattenEmailHeaders(headers []map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}

	out := make(map[string]string, len(headers))
	for _, group := range headers {
		for key, value := range group {
			out[key] = value
		}
	}

	return out
}
