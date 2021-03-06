package tokbox

import (
	"bytes"
	"net/http"
	"net/url"

	"encoding/base64"
	"encoding/json"

	"crypto/hmac"
	"crypto/sha1"

	"fmt"
	"math/rand"
	"strings"
	"time"

	"sync"

	"golang.org/x/net/context"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/google/uuid"
)

const (
	apiHost    = "https://api.opentok.com"
	apiSession = "/session/create"
	apiArchive = "/v2/project/{apiKey}/archive"
)

const (
	Days30  = 2592000 //30 * 24 * 60 * 60
	Weeks1  = 604800  //7 * 24 * 60 * 60
	Hours24 = 86400   //24 * 60 * 60
	Hours2  = 7200    //60 * 60 * 2
	Hours1  = 3600    //60 * 60
)

type MediaMode string

const (
	/**
	 * The session will send streams using the OpenTok Media Router.
	 */
	MediaRouter MediaMode = "disabled"
	/**
	* The session will attempt send streams directly between clients. If clients cannot connect
	* due to firewall restrictions, the session uses the OpenTok TURN server to relay streams.
	 */
	P2P = "enabled"
)

type ArchiveMode string

const (
	ArchiveManual  ArchiveMode = "manual"
	ArchiveAlways              = "always"
	ArchiveDisable             = "disabled"
)

type Role string

const (
	/**
	* A publisher can publish streams, subscribe to streams, and signal.
	 */
	Publisher Role = "publisher"
	/**
	* A subscriber can only subscribe to streams.
	 */
	Subscriber = "subscriber"
	/**
	* In addition to the privileges granted to a publisher, in clients using the OpenTok.js 2.2
	* library, a moderator can call the <code>forceUnpublish()</code> and
	* <code>forceDisconnect()</code> method of the Session object.
	 */
	Moderator = "moderator"
)

type LayoutType string

const (
	BestFit                LayoutType = "bestFit"
	Custom                            = "custom"
	HorizontalPresentation            = "horizontalPresentation"
	Pip                               = "pip"
	VerticalPresentation              = "verticalPresentation"
)

type OutputMode string

const (
	Composed   OutputMode = "composed"
	Individual            = "individual"
)

type Tokbox struct {
	apiKey        string
	partnerSecret string
	BetaUrl       string //Endpoint for Beta Programs
}

type Session struct {
	SessionId      string  `json:"session_id"`
	ProjectId      string  `json:"project_id"`
	PartnerId      string  `json:"partner_id"`
	CreateDt       string  `json:"create_dt"`
	SessionStatus  string  `json:"session_status"`
	MediaServerURL string  `json:"media_server_url"`
	T              *Tokbox `json:"-"`
}

type ArchiveLayout struct {
	Type            LayoutType `json:"type"`
	Stylesheet      string     `json:"stylesheet,omitempty"`
	ScreenshareType LayoutType `json:"screenshareType,omitempty"`
}

func New(apikey, partnerSecret string) *Tokbox {
	return &Tokbox{apikey, partnerSecret, ""}
}

func (t *Tokbox) jwtToken() (string, error) {

	type TokboxClaims struct {
		Ist string `json:"ist,omitempty"`
		jwt.StandardClaims
	}

	claims := TokboxClaims{
		"project",
		jwt.StandardClaims{
			Issuer:    t.apiKey,
			IssuedAt:  time.Now().UTC().Unix(),
			ExpiresAt: time.Now().UTC().Unix() + (2 * 24 * 60 * 60), // 2 hours; //NB: The maximum allowed expiration time range is 5 minutes.
			Id:        uuid.New().String(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(t.partnerSecret))
}

// Creates a new tokbox session or returns an error.
// See README file for full documentation: https://github.com/pjebs/tokbox
// NOTE: ctx must be nil if *not* using Google App Engine
func (t *Tokbox) NewSession(location string, mm MediaMode, archiveMode ArchiveMode, ctx ...context.Context) (*Session, error) {
	params := url.Values{}

	if len(location) > 0 {
		params.Add("location", location)
	}

	if len(string(archiveMode)) == 0 {
		params.Add("archiveMode", ArchiveAlways)
	} else {
		params.Add("archiveMode", string(archiveMode))
	}

	params.Add("p2p.preference", string(mm))

	var endpoint string
	if t.BetaUrl == "" {
		endpoint = apiHost
	} else {
		endpoint = t.BetaUrl
	}
	req, err := http.NewRequest("POST", endpoint+apiSession, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}

	//Create jwt token
	jwt, err := t.jwtToken()
	if err != nil {
		return nil, err
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("X-OPENTOK-AUTH", jwt)

	if len(ctx) == 0 {
		ctx = append(ctx, nil)
	}
	res, err := client(ctx[0]).Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("Tokbox returns error code: %v", res.StatusCode)
	}

	var s []Session
	if err = json.NewDecoder(res.Body).Decode(&s); err != nil {
		return nil, err
	}

	if len(s) < 1 {
		return nil, fmt.Errorf("Tokbox did not return a session")
	}

	o := s[0]
	o.T = t
	return &o, nil
}

// Customizing the video layout for composed archives
// See documentation: https://tokbox.com/developer/guides/archiving/layout-control.html
func (t *Tokbox) StartArchive(sessionId string, name string, outputMode OutputMode, layout ArchiveLayout, ctx ...context.Context) error {
	var endpoint string

	if t.BetaUrl == "" {
		endpoint = apiHost + apiArchive
	} else {
		endpoint = t.BetaUrl + apiArchive
	}

	endpoint = strings.ReplaceAll(endpoint, "{apiKey}", t.apiKey)

	params := struct {
		SessionId  string        `json:"sessionId"`
		Layout     ArchiveLayout `json:"layout"`
		Name       string        `json:"name"`
		OutputMode OutputMode    `json:"outputMode"`
	}{
		SessionId:  sessionId,
		Layout:     layout,
		Name:       name,
		OutputMode: outputMode,
	}

	data, err := json.Marshal(params)

	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(data))

	jwt, err := t.jwtToken()
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("X-OPENTOK-AUTH", jwt)

	if len(ctx) == 0 {
		ctx = append(ctx, nil)
	}
	res, err := client(ctx[0]).Do(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	if res.StatusCode != 200 {
		return fmt.Errorf("Tokbox returns error code: %v", res.StatusCode)
	}

	return nil
}

func (s *Session) Token(role Role, connectionData string, expiration int64) (string, error) {
	now := time.Now().UTC().Unix()

	dataStr := ""
	dataStr += "session_id=" + url.QueryEscape(s.SessionId)
	dataStr += "&create_time=" + url.QueryEscape(fmt.Sprintf("%d", now))
	if expiration > 0 {
		dataStr += "&expire_time=" + url.QueryEscape(fmt.Sprintf("%d", now+expiration))
	}
	if len(role) > 0 {
		dataStr += "&role=" + url.QueryEscape(string(role))
	}
	if len(connectionData) > 0 {
		dataStr += "&connection_data=" + url.QueryEscape(connectionData)
	}
	dataStr += "&nonce=" + url.QueryEscape(fmt.Sprintf("%d", rand.Intn(999999)))

	h := hmac.New(sha1.New, []byte(s.T.partnerSecret))
	n, err := h.Write([]byte(dataStr))
	if err != nil {
		return "", err
	}
	if n != len(dataStr) {
		return "", fmt.Errorf("hmac not enough bytes written %d != %d", n, len(dataStr))
	}

	preCoded := ""
	preCoded += "partner_id=" + s.T.apiKey
	preCoded += "&sig=" + fmt.Sprintf("%x:%s", h.Sum(nil), dataStr)

	var buf bytes.Buffer
	encoder := base64.NewEncoder(base64.StdEncoding, &buf)
	encoder.Write([]byte(preCoded))
	encoder.Close()
	return fmt.Sprintf("T1==%s", buf.String()), nil
}

func (s *Session) Tokens(n int, multithread bool, role Role, connectionData string, expiration int64) []string {
	ret := []string{}

	if multithread {
		var w sync.WaitGroup
		var lock sync.Mutex
		w.Add(n)

		for i := 0; i < n; i++ {
			go func(role Role, connectionData string, expiration int64) {
				a, e := s.Token(role, connectionData, expiration)
				if e == nil {
					lock.Lock()
					ret = append(ret, a)
					lock.Unlock()
				}
				w.Done()
			}(role, connectionData, expiration)

		}

		w.Wait()
		return ret
	} else {
		for i := 0; i < n; i++ {

			a, e := s.Token(role, connectionData, expiration)
			if e == nil {
				ret = append(ret, a)
			}
		}
		return ret
	}
}
