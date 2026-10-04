package backend

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dweymouth/go-jellyfin"
	"github.com/google/uuid"
	"github.com/supersonic-app/go-subsonic/subsonic"
	"github.com/supersonic-app/supersonic/backend/certs"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	jellyfinMP "github.com/supersonic-app/supersonic/backend/mediaprovider/jellyfin"
	subsonicMP "github.com/supersonic-app/supersonic/backend/mediaprovider/subsonic"
	"github.com/supersonic-app/supersonic/res"
	"github.com/zalando/go-keyring"
)

type ServerManager struct {
	LoggedInUser string
	ServerID     uuid.UUID
	Server       mediaprovider.MediaProvider

	useKeyring        bool
	prefetchCoverCB   func(string)
	appName           string
	appVersion        string
	config            *Config
	onServerConnected []func(*ServerConfig)
	onLogout          []func()

	// Client certificate (mutual TLS) state. Decrypted certificates are cached
	// for the lifetime of the process keyed by the P12 path, and the PEM files
	// materialized for mpv live in certTempDir.
	certMu      sync.Mutex
	certCache   map[string]*clientCert
	currentCert *clientCert
	certTempDir string

	// currentHTTPClient is the client most recently built for a server
	// connection, exposed so download/cache paths reuse the same TLS config.
	currentHTTPClient *http.Client
}

// clientCert holds a decrypted client identity plus the PEM files written for
// mpv (which only accepts client certificates as file paths).
type clientCert struct {
	cert     *tls.Certificate
	caFile   string
	certFile string
	keyFile  string
}

var ErrUnreachable = errors.New("server is unreachable")

func NewServerManager(appName, appVersion string, config *Config, useKeyring bool) *ServerManager {
	return &ServerManager{
		appName:    appName,
		appVersion: appVersion,
		config:     config,
		useKeyring: useKeyring,
		certCache:  make(map[string]*clientCert),
	}
}

func (s *ServerManager) SetPrefetchAlbumCoverCallback(cb func(string)) {
	s.prefetchCoverCB = cb
	if s.Server != nil {
		s.Server.SetPrefetchCoverCallback(cb)
	}
}

func (s *ServerManager) ConnectToServer(conf *ServerConfig, password string) error {
	cli, err := s.connect(conf.ServerConnection, password, conf.ID)
	if err != nil {
		return err
	}
	s.Server = cli.MediaProvider()
	s.Server.SetPrefetchCoverCallback(s.prefetchCoverCB)
	s.LoggedInUser = conf.Username
	s.ServerID = conf.ID
	s.SetDefaultServer(s.ServerID)
	for _, cb := range s.onServerConnected {
		cb(conf)
	}
	return nil
}

func (s *ServerManager) TestConnectionAndAuth(
	ctx context.Context, connection ServerConnection, password string, serverID uuid.UUID,
) error {
	err := ErrUnreachable
	done := make(chan bool)
	go func() {
		_, err = s.connect(connection, password, serverID)
		close(done)
	}()
	select {
	case <-ctx.Done():
		return err
	case <-done:
		return err
	}
}

func (s *ServerManager) GetDefaultServer() *ServerConfig {
	for _, s := range s.config.Servers {
		if s.Default {
			return s
		}
	}
	if len(s.config.Servers) > 0 {
		return s.config.Servers[0]
	}
	return nil
}

func (s *ServerManager) SetDefaultServer(serverID uuid.UUID) {
	var found bool
	for _, s := range s.config.Servers {
		f := s.ID == serverID
		if f {
			found = true
		}
		s.Default = f
	}
	if !found && len(s.config.Servers) > 0 {
		s.config.Servers[0].Default = true
	}
}

func (s *ServerManager) AddServer(nickname string, connection ServerConnection) *ServerConfig {
	sc := &ServerConfig{
		ID:               uuid.New(),
		Nickname:         nickname,
		ServerConnection: connection,
	}
	s.config.Servers = append(s.config.Servers, sc)
	return sc
}

func (s *ServerManager) DeleteServer(serverID uuid.UUID) {
	s.deleteServerPassword(serverID)
	newServers := make([]*ServerConfig, 0, len(s.config.Servers)-1)
	for _, s := range s.config.Servers {
		if s.ID != serverID {
			newServers = append(newServers, s)
		}
	}
	s.config.Servers = newServers
}

func (s *ServerManager) Logout(deletePassword bool) {
	if s.Server != nil {
		if deletePassword {
			s.deleteServerPassword(s.ServerID)
		}
		for _, cb := range s.onLogout {
			cb()
		}
		s.Server = nil
		s.LoggedInUser = ""
		s.ServerID = uuid.UUID{}
	}
}

func (s *ServerManager) deleteServerPassword(serverID uuid.UUID) {
	if s.useKeyring {
		keyring.Delete(s.appName, serverID.String())
		keyring.Delete(s.appName, certPassphraseKey(serverID))
	}
}

// Sets a callback that is invoked when a server is connected to.
func (s *ServerManager) OnServerConnected(cb func(*ServerConfig)) {
	s.onServerConnected = append(s.onServerConnected, cb)
}

// Sets a callback that is invoked when the user logs out of a server.
func (s *ServerManager) OnLogout(cb func()) {
	s.onLogout = append(s.onLogout, cb)
}

func (s *ServerManager) GetServerPassword(serverID uuid.UUID) (string, error) {
	if s.useKeyring {
		return keyring.Get(s.appName, serverID.String())
	}
	return "", errors.New("keyring not enabled")
}

func (s *ServerManager) SetServerPassword(server *ServerConfig, password string) error {
	if s.useKeyring {
		return keyring.Set(s.appName, server.ID.String(), password)
	}
	return errors.New("keyring not available")
}

// certPassphraseKey is the keyring entry name for a server's client certificate
// passphrase. Kept separate from the account password entry.
func certPassphraseKey(serverID uuid.UUID) string {
	return serverID.String() + "/cert"
}

// GetServerCertPassphrase fetches a server's client certificate passphrase from
// the OS keyring.
func (s *ServerManager) GetServerCertPassphrase(serverID uuid.UUID) (string, error) {
	if s.useKeyring {
		return keyring.Get(s.appName, certPassphraseKey(serverID))
	}
	return "", errors.New("keyring not enabled")
}

// SetServerCertPassphrase stores a server's client certificate passphrase in the
// OS keyring so it survives restarts, like the account password.
func (s *ServerManager) SetServerCertPassphrase(serverID uuid.UUID, passphrase string) error {
	if s.useKeyring {
		return keyring.Set(s.appName, certPassphraseKey(serverID), passphrase)
	}
	return errors.New("keyring not available")
}

func (s *ServerManager) connect(connection ServerConnection, password string, serverID uuid.UUID) (mediaprovider.Server, error) {
	var cli, altCli mediaprovider.Server
	timeout := time.Second * time.Duration(s.config.Application.RequestTimeoutSeconds)

	if connection.ServerType == ServerTypeJellyfin {
		connection.Hostname = NormalizeJellyfinURL(connection.Hostname)
		connection.AltHostname = NormalizeJellyfinURL(connection.AltHostname)
	} else {
		connection.Hostname = NormalizeServerURL(connection.Hostname)
		connection.AltHostname = NormalizeServerURL(connection.AltHostname)
	}

	// Retrieve the client certificate passphrase from the keyring when the
	// connection doesn't carry one (e.g. automatic reconnect after restart).
	if connection.ClientCertPath != "" && connection.ClientCertPassphrase == "" && serverID != uuid.Nil {
		if pass, err := s.GetServerCertPassphrase(serverID); err == nil {
			connection.ClientCertPassphrase = pass
		}
	}

	httpClient, err := s.buildHTTPClient(connection, timeout, serverID)
	if err != nil {
		return nil, err
	}

	if connection.ServerType == ServerTypeJellyfin {
		client, err := jellyfin.NewClient(connection.Hostname, res.AppName, res.AppVersion, jellyfin.WithHTTPClient(httpClient))
		if err != nil {
			log.Printf("error creating Jellyfin client: %s", err.Error())
			return nil, err
		}
		cli = &jellyfinMP.JellyfinServer{
			Client: *client,
		}

		if connection.AltHostname != "" {
			altClient, err := jellyfin.NewClient(connection.AltHostname, res.AppName, res.AppVersion, jellyfin.WithHTTPClient(httpClient))
			if err != nil {
				log.Printf("error creating Jellyfin alternative client: %s", err.Error())
				return nil, err
			}
			altCli = &jellyfinMP.JellyfinServer{
				Client: *altClient,
			}
		}
	} else {
		ua := fmt.Sprintf("%s/%s", s.appName, s.appVersion)
		cli = &subsonicMP.SubsonicServer{
			Client: subsonic.Client{
				UserAgent:    ua,
				Client:       httpClient,
				BaseUrl:      connection.Hostname,
				User:         connection.Username,
				PasswordAuth: connection.LegacyAuth,
				ClientName:   res.AppName,
				UseJSON:      true,
			},
		}
		altCli = &subsonicMP.SubsonicServer{
			Client: subsonic.Client{
				UserAgent:    ua,
				Client:       httpClient,
				BaseUrl:      connection.AltHostname,
				User:         connection.Username,
				PasswordAuth: connection.LegacyAuth,
				ClientName:   res.AppName,
				UseJSON:      true,
			},
		}
	}

	// struct to return hostname type in isAlt and connection success on err
	type pingResult struct {
		isAlt bool
		err   error
	}
	pingChan := make(chan pingResult, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pingFunc := func(delay time.Duration, cli mediaprovider.Server, isAlt bool) {
		// delay before connecting or exit if already cancelled
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}

		resp := cli.Login(connection.Username, password)
		if resp.Error != nil && !resp.IsAuthError {
			return
		}

		// return result or exit if already cancelled
		select {
		case pingChan <- pingResult{isAlt: isAlt, err: resp.Error}:
		case <-ctx.Done():
		}
	}
	go pingFunc(0, cli, false)
	if connection.AltHostname != "" {
		go pingFunc(333*time.Millisecond, altCli, true) // give primary hostname ping a head start
	}

	select {
	case <-ctx.Done():
		return nil, ErrUnreachable
	case res := <-pingChan:
		if res.isAlt {
			return altCli, res.err
		}
		return cli, res.err
	}
}

// buildHTTPClient builds an HTTP client for the given server connection,
// applying the client certificate and server-trust settings (skip-verify and
// optional CA bundle) on a single shared TLS configuration.
func (s *ServerManager) buildHTTPClient(connection ServerConnection, timeout time.Duration, serverID uuid.UUID) (*http.Client, error) {
	tlsConfig, err := s.buildTLSConfig(connection, serverID)
	if err != nil {
		return nil, err
	}
	cli := &http.Client{Timeout: timeout}
	if tlsConfig != nil {
		cli.Transport = &http.Transport{TLSClientConfig: tlsConfig}
	}
	s.certMu.Lock()
	s.currentHTTPClient = cli
	s.certMu.Unlock()
	return cli, nil
}

func (s *ServerManager) buildTLSConfig(connection ServerConnection, serverID uuid.UUID) (*tls.Config, error) {
	if !connection.SkipSSLVerify && connection.ClientCertPath == "" && connection.ClientCertCAFile == "" {
		return nil, nil
	}
	cfg := &tls.Config{}
	if connection.SkipSSLVerify {
		cfg.InsecureSkipVerify = true
	}
	if connection.ClientCertCAFile != "" {
		pemBytes, err := os.ReadFile(connection.ClientCertCAFile)
		if err != nil {
			return nil, fmt.Errorf("reading CA certificate %q: %w", connection.ClientCertCAFile, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, fmt.Errorf("no certificates found in CA file %q", connection.ClientCertCAFile)
		}
		cfg.RootCAs = pool
	}
	if connection.ClientCertPath != "" {
		cc, err := s.loadClientCert(connection, serverID)
		if err != nil {
			return nil, err
		}
		cfg.Certificates = []tls.Certificate{*cc.cert}
	}
	return cfg, nil
}

// loadClientCert decrypts the server's PKCS#12 client certificate, caching the
// result for the process lifetime and materializing PEM files for mpv.
func (s *ServerManager) loadClientCert(connection ServerConnection, serverID uuid.UUID) (*clientCert, error) {
	key := connection.ClientCertPath
	s.certMu.Lock()
	if cc, ok := s.certCache[key]; ok {
		s.currentCert = cc
		s.certMu.Unlock()
		return cc, nil
	}
	s.certMu.Unlock()

	cert, err := certs.LoadPKCS12(connection.ClientCertPath, connection.ClientCertPassphrase)
	if err != nil {
		return nil, err
	}

	dir, err := s.ensureCertTempDir()
	if err != nil {
		return nil, err
	}
	certFile, keyFile, err := certs.WriteTempPEM(cert, dir, certFileStem(serverID, connection.ClientCertPath))
	if err != nil {
		return nil, fmt.Errorf("writing client certificate for playback: %w", err)
	}

	cc := &clientCert{
		cert:     cert,
		caFile:   connection.ClientCertCAFile,
		certFile: certFile,
		keyFile:  keyFile,
	}
	s.certMu.Lock()
	s.certCache[key] = cc
	s.currentCert = cc
	s.certMu.Unlock()
	return cc, nil
}

// certFileStem names the materialized PEM files. A non-nil server ID is used so
// the name never leaks the certificate's source path; the transient pre-add
// test (uuid.Nil) falls back to a short hash of the path.
func certFileStem(serverID uuid.UUID, path string) string {
	if serverID != uuid.Nil {
		return serverID.String()
	}
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:8])
}

// certTempBase returns the directory under which per-process client-cert PEM
// files are created. It prefers XDG_RUNTIME_DIR (tmpfs, user-only on Linux) and
// otherwise falls back to the OS temp directory.
func certTempBase() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	return os.TempDir()
}

func (s *ServerManager) ensureCertTempDir() (string, error) {
	s.certMu.Lock()
	defer s.certMu.Unlock()
	if s.certTempDir != "" {
		return s.certTempDir, nil
	}
	base := certTempBase()
	sweepStaleCertTempDirs(base)
	dir, err := os.MkdirTemp(base, "supersonic-certs-")
	if err != nil {
		return "", err
	}
	s.certTempDir = dir
	return dir, nil
}

// sweepStaleCertTempDirs removes leftover per-process cert dirs from previous
// runs. Only dirs older than a day are removed so a concurrently running
// instance (multi-instance mode) is never disturbed.
func sweepStaleCertTempDirs(base string) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "supersonic-certs-") {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		os.RemoveAll(filepath.Join(base, e.Name()))
	}
}

// ClientCertFiles returns the PEM files (cert, key) and CA file currently in
// use for the connected server, or empty strings if no client certificate is
// configured. It is used to configure mpv playback.
func (s *ServerManager) ClientCertFiles() (certFile, keyFile, caFile string) {
	s.certMu.Lock()
	defer s.certMu.Unlock()
	if s.currentCert == nil {
		return "", "", ""
	}
	return s.currentCert.certFile, s.currentCert.keyFile, s.currentCert.caFile
}

// CleanupClientCertFiles removes the materialized PEM files on shutdown.
func (s *ServerManager) CleanupClientCertFiles() {
	s.certMu.Lock()
	dir := s.certTempDir
	s.certMu.Unlock()
	if dir != "" {
		os.RemoveAll(dir)
	}
}

// HTTPClient returns the HTTP client most recently built for a server
// connection, or nil if none has been built. Used by download/cache paths so
// they honour the same client certificate and TLS settings as the API clients.
func (s *ServerManager) HTTPClient() *http.Client {
	s.certMu.Lock()
	defer s.certMu.Unlock()
	return s.currentHTTPClient
}

func (a *ServerManager) GetServer() mediaprovider.MediaProvider {
	return a.Server
}

// NormalizeServerURL applies common normalization to a server URL:
// prepends "http://" if no scheme is present, then strips trailing slashes.
func NormalizeServerURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	if !strings.Contains(rawURL, "://") {
		rawURL = "http://" + rawURL
	}
	rawURL = strings.TrimRight(rawURL, "/")
	return rawURL
}

// NormalizeJellyfinURL applies common normalization then additionally strips
// known Jellyfin web UI path suffixes (/web/index.html and /web).
func NormalizeJellyfinURL(rawURL string) string {
	rawURL = NormalizeServerURL(rawURL)
	if strings.HasSuffix(rawURL, "/web/index.html") {
		rawURL = strings.TrimSuffix(rawURL, "/web/index.html")
	} else if strings.HasSuffix(rawURL, "/web") {
		rawURL = strings.TrimSuffix(rawURL, "/web")
	}
	return rawURL
}
