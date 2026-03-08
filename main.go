package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type contextKey string

const userContextKey contextKey = "user"

type LockConfig struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`        // TOGGLE / STRIKE / PULSE
	HasBattery bool   `json:"has_battery"` // true / false
}

type ACLRule struct {
	User  string   `json:"user"`  // "*" for all users
	Locks []string `json:"locks"` // ["*"] or list of IDs
}

type Config struct {
	MQTT struct {
		Broker      string `json:"broker"`
		Port        int    `json:"port"`
		Username    string `json:"username"` // Base64 encoded
		Password    string `json:"password"` // Base64 encoded
		CAFile      string `json:"ca_file"`
		ClientID    string `json:"client_id"`
		TopicState  string `json:"topic_state"`   // locks/+/state
		TopicBatt   string `json:"topic_batt"`    // locks/+/batt
		TopicCmdTpl string `json:"topic_cmd_tpl"` // locks/%s/cmd
	} `json:"mqtt"`
	HTTP struct {
		Listen    string `json:"listen"`     // 127.0.0.1:8884
		AuthFile  string `json:"auth_file"`  // /etc/lockd/auth_keys
		AuditFile string `json:"audit_file"` // /etc/lockd/audit.log
		CertFile  string `json:"cert_file"`  // path to cert
		KeyFile   string `json:"key_file"`   // path to key
	} `json:"http"`
	Locks []LockConfig `json:"locks"`
	ACL   []ACLRule    `json:"acl"`
}

type State struct {
	LockID    string    `json:"lock_id"`
	State     string    `json:"state"`      // Nyitva / Zárva / Ismeretlen...
	Battery   string    `json:"battery"`    // %
	UpdatedAt time.Time `json:"updated_at"` // server time
}

type LockResponse struct {
	LockConfig
	State     string    `json:"state,omitempty"`
	Battery   string    `json:"battery,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type Server struct {
	cfg     Config
	mc      mqtt.Client
	mu      sync.RWMutex
	state   map[string]State
	tlsCert *tls.Certificate // Cached certificate for reloading
}

func decodeB64(s string) string {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return s
	}
	return string(b)
}

func loadConfig(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, err
	}

	if c.HTTP.Listen == "" {
		c.HTTP.Listen = "127.0.0.1:8884"
	}
	if c.HTTP.AuthFile == "" {
		c.HTTP.AuthFile = "/etc/lockd/auth_keys"
	}
	if c.HTTP.AuditFile == "" {
		c.HTTP.AuditFile = "/etc/lockd/audit.log"
	}

	c.MQTT.Username = decodeB64(c.MQTT.Username)
	c.MQTT.Password = decodeB64(c.MQTT.Password)

	return c, nil
}

func tlsConfigFromCA(caFile string) (*tls.Config, error) {
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM(pem); !ok {
		return nil, errors.New("CA PEM parse failed")
	}
	return &tls.Config{
		RootCAs:            pool,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: false,
	}, nil
}

func (s *Server) updateStateFromTopic(topic, payload string) {
	parts := strings.Split(topic, "/")
	if len(parts) < 3 {
		return
	}
	lockID := parts[1]
	suffix := parts[2]

	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.state[lockID]
	if !ok {
		st = State{LockID: lockID}
	}

	if suffix == "state" {
		st.State = payload
	} else if suffix == "batt" {
		st.Battery = payload
	}

	st.UpdatedAt = time.Now()
	s.state[lockID] = st
}

func (s *Server) reloadCert() error {
	s.mu.RLock()
	certFile := s.cfg.HTTP.CertFile
	keyFile := s.cfg.HTTP.KeyFile
	s.mu.RUnlock()

	if certFile == "" || keyFile == "" {
		return nil
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.tlsCert = &cert
	s.mu.Unlock()
	return nil
}

func (s *Server) getCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.tlsCert == nil {
		return nil, errors.New("no certificate loaded")
	}
	return s.tlsCert, nil
}

func (s *Server) mqttConnect() {
	tlsCfg, err := tlsConfigFromCA(s.cfg.MQTT.CAFile)
	if err != nil {
		log.Fatalf("tls ca load failed: %v", err)
	}

	brokerURL := fmt.Sprintf("ssl://%s:%d", s.cfg.MQTT.Broker, s.cfg.MQTT.Port)

	opts := mqtt.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID(s.cfg.MQTT.ClientID).
		SetUsername(s.cfg.MQTT.Username).
		SetPassword(s.cfg.MQTT.Password).
		SetTLSConfig(tlsCfg).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(3 * time.Second).
		SetOnConnectHandler(func(c mqtt.Client) {
			log.Printf("MQTT connected (as %s): %s", s.cfg.MQTT.ClientID, brokerURL)
			c.Subscribe(s.cfg.MQTT.TopicState, 1, func(_ mqtt.Client, m mqtt.Message) {
				s.updateStateFromTopic(m.Topic(), string(m.Payload()))
			})
			c.Subscribe(s.cfg.MQTT.TopicBatt, 1, func(_ mqtt.Client, m mqtt.Message) {
				s.updateStateFromTopic(m.Topic(), string(m.Payload()))
			})
		})

	s.mc = mqtt.NewClient(opts)
	if tok := s.mc.Connect(); tok.Wait() && tok.Error() != nil {
		log.Fatalf("MQTT connect failed: %v", tok.Error())
	}
}

func (s *Server) auditLog(user, lockID, cmd string) {
	msg := fmt.Sprintf("%s | User: %s | Lock: %s | Cmd: %s\n", 
		time.Now().Format("2006-01-02 15:04:05"), user, lockID, cmd)
	
	f, err := os.OpenFile(s.cfg.HTTP.AuditFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("audit log error: %v", err)
		return
	}
	defer f.Close()
	_, _ = f.WriteString(msg)
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		if key == "" { key = r.URL.Query().Get("key") }
		if key == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		hashBytes := sha256.Sum256([]byte(key))
		keyHash := hex.EncodeToString(hashBytes[:])

		f, err := os.Open(s.cfg.HTTP.AuthFile)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer f.Close()

		username := ""
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") { continue }
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 && parts[1] == keyHash {
				username = parts[0]
				break
			}
		}

		if username == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, username)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) canAccess(user, lockID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.cfg.ACL) == 0 {
		return true
	}

	for _, rule := range s.cfg.ACL {
		if rule.User == user || rule.User == "*" {
			for _, id := range rule.Locks {
				if id == lockID || id == "*" {
					return true
				}
			}
		}
	}
	return false
}

func (s *Server) handleLocks(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, _ := r.Context().Value(userContextKey).(string)

	out := make([]LockResponse, 0, len(s.cfg.Locks))
	for _, lc := range s.cfg.Locks {
		if !s.canAccess(user, lc.ID) {
			continue
		}
		lr := LockResponse{LockConfig: lc}
		if st, ok := s.state[lc.ID]; ok {
			lr.State = st.State
			lr.Battery = st.Battery
			lr.UpdatedAt = st.UpdatedAt
		}
		out = append(out, lr)
	}
	
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"locks": out})
}

func (s *Server) handleCmd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	p := strings.TrimPrefix(r.URL.Path, "/v1/locks/")
	p = strings.Trim(p, "/")
	parts := strings.Split(p, "/")
	if len(parts) != 2 || parts[1] != "cmd" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]

	var req struct { Cmd string `json:"cmd"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	cmd := strings.ToUpper(strings.TrimSpace(req.Cmd))

	user, _ := r.Context().Value(userContextKey).(string)
	if !s.canAccess(user, id) {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}

	// Find lock config for validation
	s.mu.RLock()
	var targetLock *LockConfig
	for i := range s.cfg.Locks {
		if s.cfg.Locks[i].ID == id { targetLock = &s.cfg.Locks[i]; break }
	}
	s.mu.RUnlock()

	if targetLock == nil {
		http.Error(w, "unknown lock", http.StatusNotFound)
		return
	}

	// Guard: no LOCK for non-TOGGLE types (STRIKE, PULSE, OPEN)
	if targetLock.Type != "TOGGLE" && cmd == "LOCK" {
		http.Error(w, "LOCK not supported for this type", http.StatusBadRequest)
		return
	}

	topic := fmt.Sprintf(s.cfg.MQTT.TopicCmdTpl, id)
	tok := s.mc.Publish(topic, 1, false, cmd)
	tok.Wait()
	if tok.Error() != nil {
		http.Error(w, tok.Error().Error(), http.StatusInternalServerError)
		return
	}

	// Audit log using the user we already extracted
	s.auditLog(user, id, cmd)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func main() {
	var (
		cfgPath = flag.String("config", "/etc/lockd2.json", "Path to the JSON configuration file")
		encode  = flag.String("encode", "", "Helper to Base64 encode a string (useful for MQTT username/password in config)")
		genKey  = flag.String("gen-key", "", "Generate a new API key for a 'username' (outputs raw key for app and hash for auth_keys)")
	)
	flag.Parse()

	if *encode != "" {
		fmt.Println(base64.StdEncoding.EncodeToString([]byte(*encode)))
		return
	}

	if *genKey != "" {
		key := fmt.Sprintf("%d-%x", time.Now().UnixNano(), sha256.Sum256([]byte(*genKey)))
		key = key[:32]
		hash := hex.EncodeToString(sha256.New().Sum([]byte(key))) // simplified for example
		// correcting the above:
		hRaw := sha256.Sum256([]byte(key))
		hash = hex.EncodeToString(hRaw[:])
		fmt.Println("--- NEW API KEY GENERATED ---")
		fmt.Printf("User: %s\n", *genKey)
		fmt.Printf("Raw Key: %s  <-- COPY THIS into the Mobile App\n", key)
		fmt.Printf("Auth Line: %s:%s  <-- APPEND THIS to your 'auth_keys' file\n", *genKey, hash)
		fmt.Println("-----------------------------")
		return
	}

	cfg, err := loadConfig(*cfgPath)
	if err != nil { log.Fatalf("config error: %v", err) }

	// Ensure AuthFile exists (create dir and file if missing)
	if cfg.HTTP.AuthFile != "" {
		_ = os.MkdirAll(filepath.Dir(cfg.HTTP.AuthFile), 0755)
		if _, err := os.Stat(cfg.HTTP.AuthFile); os.IsNotExist(err) {
			log.Printf("Auth file missing, creating: %s", cfg.HTTP.AuthFile)
			_ = os.WriteFile(cfg.HTTP.AuthFile, []byte("# lockd2 auth keys\n"), 0600)
		}
	}
	// Ensure AuditFile directory exists
	if cfg.HTTP.AuditFile != "" {
		_ = os.MkdirAll(filepath.Dir(cfg.HTTP.AuditFile), 0755)
	}

	s := &Server{
		cfg:   cfg,
		state: make(map[string]State),
	}
	s.mqttConnect()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if s.mc == nil || !s.mc.IsConnected() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("mqtt disconnected\n"))
			return
		}
		_, _ = w.Write([]byte("ok\n"))
	})

	api := http.NewServeMux()
	api.HandleFunc("/v1/locks", s.handleLocks)
	api.HandleFunc("/v1/locks/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/cmd") { s.handleCmd(w, r); return }
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	mux.Handle("/v1/", s.auth(api))

	tlsCfg := &tls.Config{
		GetCertificate: s.getCertificate,
		MinVersion:     tls.VersionTLS12,
	}
	srv := &http.Server{
		Addr:      cfg.HTTP.Listen,
		Handler:   mux,
		TLSConfig: tlsCfg,
	}

	// Initial cert load
	if cfg.HTTP.CertFile != "" && cfg.HTTP.KeyFile != "" {
		if err := s.reloadCert(); err != nil {
			log.Fatalf("initial cert load failed: %v", err)
		}
	}

	// SIGHUP Reload
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGHUP)
		for range sig {
			log.Printf("SIGHUP received, reloading config and certificates...")
			newCfg, err := loadConfig(*cfgPath)
			if err != nil {
				log.Printf("reload failed: %v", err)
				continue
			}
			s.mu.Lock()
			s.cfg = newCfg
			s.mu.Unlock()

			if newCfg.HTTP.CertFile != "" && newCfg.HTTP.KeyFile != "" {
				if err := s.reloadCert(); err != nil {
					log.Printf("cert reload failed: %v", err)
				} else {
					log.Printf("certificates reloaded")
				}
			}
			log.Printf("config reloaded")
		}
	}()

	go func() {
		if cfg.HTTP.CertFile != "" && cfg.HTTP.KeyFile != "" {
			log.Printf("HTTPS listening on %s (dynamic reload enabled)", cfg.HTTP.Listen)
			if err := srv.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("https server failed: %v", err)
			}
		} else {
			log.Printf("HTTP listening on %s (WARNING: plain HTTP)", cfg.HTTP.Listen)
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("http server failed: %v", err)
			}
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Printf("shutting down...")
	_ = srv.Shutdown(context.Background())
	if s.mc != nil { s.mc.Disconnect(250) }
}
