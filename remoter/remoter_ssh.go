package remoter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/melbahja/goph"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/andrewbytecoder/gokit/options"
)

const (
	defaultSSHPort    = 22
	defaultSSHTimeout = 20 * time.Second
)

// Remoter is the concrete implementation of IRemoter using goph.
// It supports SSH connections via password, private key, or ssh-agent,
// and provides SFTP-based file operations.
type Remoter struct {
	Host     string
	Port     int
	User     string
	Password string

	PrivateKeyPath        string
	PrivateKeyPassphrase  string
	KnownHostsPath        string
	InsecureIgnoreHostKey bool
	Timeout               time.Duration

	mu         sync.Mutex
	client     *goph.Client
	sftpClient *sftp.Client
}

var _ IRemoter = (*Remoter)(nil)

// SetHost is an option to set the remote host address.
func SetHost(host string) options.Option {
	return func(o interface{}) {
		o.(*Remoter).Host = host
	}
}

// SetPort is an option to set the remote SSH port.
func SetPort(port int) options.Option {
	return func(o interface{}) {
		o.(*Remoter).Port = port
	}
}

// SetUser is an option to set the SSH login user.
func SetUser(user string) options.Option {
	return func(o interface{}) {
		o.(*Remoter).User = user
	}
}

// SetPassword is an option to set the SSH password for authentication.
func SetPassword(password string) options.Option {
	return func(o interface{}) {
		o.(*Remoter).Password = password
	}
}

// SetPrivateKeyPath is an option to set the path to the SSH private key file.
func SetPrivateKeyPath(privateKeyPath string) options.Option {
	return func(o interface{}) {
		o.(*Remoter).PrivateKeyPath = privateKeyPath
	}
}

// SetPrivateKeyPassphrase is an option to set the passphrase for the private key.
func SetPrivateKeyPassphrase(passphrase string) options.Option {
	return func(o interface{}) {
		o.(*Remoter).PrivateKeyPassphrase = passphrase
	}
}

// SetKnownHostsPath is an option to set the path to the known_hosts file.
func SetKnownHostsPath(knownHostsPath string) options.Option {
	return func(o interface{}) {
		o.(*Remoter).KnownHostsPath = knownHostsPath
	}
}

// SetInsecureIgnoreHostKey is an option to skip host key verification.
// Use only for testing; it disables MITM protection.
func SetInsecureIgnoreHostKey(ignore bool) options.Option {
	return func(o interface{}) {
		o.(*Remoter).InsecureIgnoreHostKey = ignore
	}
}

// SetTimeout is an option to set the SSH connection timeout.
func SetTimeout(timeout time.Duration) options.Option {
	return func(o interface{}) {
		o.(*Remoter).Timeout = timeout
	}
}

// NewRemoter creates a new Remoter instance with default port (22)
// and default timeout (20s). Use option functions to override defaults.
func NewRemoter(opts ...options.Option) *Remoter {
	r := &Remoter{
		Port:    defaultSSHPort,
		Timeout: defaultSSHTimeout,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Connect establishes the SSH connection. It is safe to call multiple times;
// subsequent calls are no-ops if the connection is already established.
func (r *Remoter) Connect() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.client != nil {
		return nil
	}

	config, err := r.buildConfig()
	if err != nil {
		return err
	}

	client, err := goph.NewConn(config)
	if err != nil {
		return fmt.Errorf("connect ssh %s:%d failed: %w", r.Host, r.Port, err)
	}

	r.client = client
	return nil
}

// Close terminates the SSH connection and releases the SFTP client.
// It is safe to call multiple times.
func (r *Remoter) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var closeErr error
	if r.sftpClient != nil {
		if err := r.sftpClient.Close(); err != nil {
			closeErr = errors.Join(closeErr, err)
		}
		r.sftpClient = nil
	}
	if r.client != nil {
		if err := r.client.Close(); err != nil {
			closeErr = errors.Join(closeErr, err)
		}
		r.client = nil
	}
	return closeErr
}

// RunCmd executes a command on the remote host and returns the combined
// stdout and stderr output. It auto-connects if not already connected.
func (r *Remoter) RunCmd(cmd string) ([]byte, error) {
	client, err := r.ensureClient()
	if err != nil {
		return nil, err
	}
	out, err := client.Run(cmd)
	if err != nil {
		return out, fmt.Errorf("run remote command failed: %w", err)
	}
	return out, nil
}

// RunCmdContext is like RunCmd but accepts a context for timeout and cancellation.
func (r *Remoter) RunCmdContext(ctx context.Context, cmd string) ([]byte, error) {
	client, err := r.ensureClient()
	if err != nil {
		return nil, err
	}
	out, err := client.RunContext(ctx, cmd)
	if err != nil {
		return out, fmt.Errorf("run remote command with context failed: %w", err)
	}
	return out, nil
}

// UploadFile uploads a single local file to the remote host via SFTP.
// For directory sync, use UploadDir instead.
func (r *Remoter) UploadFile(localPath, remotePath string) error {
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("stat local file %q failed: %w", localPath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("local path %q is a directory", localPath)
	}

	return r.uploadSingleFile(localPath, cleanRemotePath(remotePath), info.Mode().Perm())
}

// UploadDir recursively syncs an entire local directory to the remote host via SFTP.
// It creates the remote directory tree and uploads all files with their original permissions.
func (r *Remoter) UploadDir(localDir, remoteDir string) error {
	info, err := os.Stat(localDir)
	if err != nil {
		return fmt.Errorf("stat local dir %q failed: %w", localDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("local path %q is not a directory", localDir)
	}

	sftpClient, err := r.ensureSFTP()
	if err != nil {
		return err
	}

	remoteDir = cleanRemotePath(remoteDir)
	if err := sftpClient.MkdirAll(remoteDir); err != nil {
		return fmt.Errorf("create remote dir %q failed: %w", remoteDir, err)
	}

	return filepath.Walk(localDir, func(current string, currentInfo os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(localDir, current)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		target := joinRemotePath(remoteDir, filepath.ToSlash(rel))
		if currentInfo.IsDir() {
			if err := sftpClient.MkdirAll(target); err != nil {
				return fmt.Errorf("create remote dir %q failed: %w", target, err)
			}
			return nil
		}

		return r.uploadSingleFile(current, target, currentInfo.Mode().Perm())
	})
}

// DownloadFile downloads a single remote file to the local machine via SFTP.
// Local parent directories are created automatically if they do not exist.
func (r *Remoter) DownloadFile(remotePath, localPath string) error {
	sftpClient, err := r.ensureSFTP()
	if err != nil {
		return err
	}

	remotePath = cleanRemotePath(remotePath)
	remoteFile, err := sftpClient.Open(remotePath)
	if err != nil {
		return fmt.Errorf("open remote file %q failed: %w", remotePath, err)
	}
	defer remoteFile.Close()

	info, err := remoteFile.Stat()
	if err != nil {
		return fmt.Errorf("stat remote file %q failed: %w", remotePath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("remote path %q is a directory", remotePath)
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return fmt.Errorf("create local parent dir for %q failed: %w", localPath, err)
	}

	localFile, err := os.OpenFile(localPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, normalizePerm(info.Mode()))
	if err != nil {
		return fmt.Errorf("open local file %q failed: %w", localPath, err)
	}
	defer localFile.Close()

	if _, err := io.Copy(localFile, remoteFile); err != nil {
		return fmt.Errorf("download %q to %q failed: %w", remotePath, localPath, err)
	}

	if err := localFile.Sync(); err != nil {
		return fmt.Errorf("sync local file %q failed: %w", localPath, err)
	}
	return nil
}

// PathExists checks whether a file or directory exists at the given remote path.
func (r *Remoter) PathExists(remotePath string) (bool, error) {
	sftpClient, err := r.ensureSFTP()
	if err != nil {
		return false, err
	}

	_, err = sftpClient.Stat(cleanRemotePath(remotePath))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("stat remote path %q failed: %w", remotePath, err)
}

// DeleteFile deletes a single file on the remote host. Returns an error if the path is a directory.
func (r *Remoter) DeleteFile(remotePath string) error {
	sftpClient, err := r.ensureSFTP()
	if err != nil {
		return err
	}

	remotePath = cleanRemotePath(remotePath)
	info, err := sftpClient.Stat(remotePath)
	if err != nil {
		return fmt.Errorf("stat remote file %q failed: %w", remotePath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("remote path %q is a directory", remotePath)
	}
	if err := sftpClient.Remove(remotePath); err != nil {
		return fmt.Errorf("delete remote file %q failed: %w", remotePath, err)
	}
	return nil
}

// DeleteDir recursively deletes a directory and all its contents on the remote host.
// Returns an error if the path is not a directory.
func (r *Remoter) DeleteDir(remotePath string) error {
	sftpClient, err := r.ensureSFTP()
	if err != nil {
		return err
	}

	remotePath = cleanRemotePath(remotePath)
	info, err := sftpClient.Stat(remotePath)
	if err != nil {
		return fmt.Errorf("stat remote dir %q failed: %w", remotePath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("remote path %q is not a directory", remotePath)
	}
	if err := sftpClient.RemoveAll(remotePath); err != nil {
		return fmt.Errorf("delete remote dir %q failed: %w", remotePath, err)
	}
	return nil
}

// WriteFile writes content to a remote file, creating parent directories as needed.
// It uses an atomic write-via-tempfile approach: writes to a temp file first,
// then renames it to the target path. Creates the file if absent, overwrites if present.
func (r *Remoter) WriteFile(remotePath string, content []byte, perm os.FileMode) error {
	sftpClient, err := r.ensureSFTP()
	if err != nil {
		return err
	}

	remotePath = cleanRemotePath(remotePath)
	parentDir := path.Dir(remotePath)
	if err := sftpClient.MkdirAll(parentDir); err != nil {
		return fmt.Errorf("create remote parent dir for %q failed: %w", remotePath, err)
	}

	tmpPath := fmt.Sprintf("%s.tmp.%d", remotePath, time.Now().UnixNano())
	tmpFile, err := sftpClient.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("open temp remote file %q failed: %w", tmpPath, err)
	}

	writeErr := func() error {
		if _, err := tmpFile.Write(content); err != nil {
			return fmt.Errorf("write temp remote file %q failed: %w", tmpPath, err)
		}
		if err := tmpFile.Chmod(normalizePerm(perm)); err != nil {
			return fmt.Errorf("chmod temp remote file %q failed: %w", tmpPath, err)
		}
		if err := tmpFile.Close(); err != nil {
			return fmt.Errorf("close temp remote file %q failed: %w", tmpPath, err)
		}
		if err := r.renameRemoteFile(sftpClient, tmpPath, remotePath); err != nil {
			return fmt.Errorf("rename temp remote file %q to %q failed: %w", tmpPath, remotePath, err)
		}
		return nil
	}()
	if writeErr != nil {
		_ = tmpFile.Close()
		_ = sftpClient.Remove(tmpPath)
		return writeErr
	}
	return nil
}

// ReadFile reads the entire content of a remote file via SFTP.
func (r *Remoter) ReadFile(remotePath string) ([]byte, error) {
	sftpClient, err := r.ensureSFTP()
	if err != nil {
		return nil, err
	}

	remotePath = cleanRemotePath(remotePath)
	remoteFile, err := sftpClient.Open(remotePath)
	if err != nil {
		return nil, fmt.Errorf("open remote file %q failed: %w", remotePath, err)
	}
	defer remoteFile.Close()

	content, err := io.ReadAll(remoteFile)
	if err != nil {
		return nil, fmt.Errorf("read remote file %q failed: %w", remotePath, err)
	}
	return content, nil
}

func (r *Remoter) buildConfig() (*goph.Config, error) {
	if strings.TrimSpace(r.Host) == "" {
		return nil, errors.New("ssh host is required")
	}
	if strings.TrimSpace(r.User) == "" {
		return nil, errors.New("ssh user is required")
	}

	auth, err := r.buildAuth()
	if err != nil {
		return nil, err
	}

	callback, err := r.buildHostKeyCallback()
	if err != nil {
		return nil, err
	}

	port := r.Port
	if port <= 0 {
		port = defaultSSHPort
	}

	timeout := r.Timeout
	if timeout <= 0 {
		timeout = defaultSSHTimeout
	}

	return &goph.Config{
		User:     r.User,
		Addr:     r.Host,
		Port:     uint(port),
		Auth:     auth,
		Timeout:  timeout,
		Callback: callback,
	}, nil
}

func (r *Remoter) buildAuth() (goph.Auth, error) {
	if strings.TrimSpace(r.PrivateKeyPath) != "" {
		return goph.Key(r.PrivateKeyPath, r.PrivateKeyPassphrase)
	}
	if r.Password != "" {
		return goph.KeyboardInteractive(r.Password), nil
	}
	if goph.HasAgent() {
		return goph.UseAgent()
	}
	return nil, errors.New("no ssh auth method configured")
}

func (r *Remoter) buildHostKeyCallback() (ssh.HostKeyCallback, error) {
	if r.InsecureIgnoreHostKey {
		return ssh.InsecureIgnoreHostKey(), nil
	}
	if strings.TrimSpace(r.KnownHostsPath) != "" {
		return goph.KnownHosts(r.KnownHostsPath)
	}
	return goph.DefaultKnownHosts()
}

func (r *Remoter) ensureClient() (*goph.Client, error) {
	r.mu.Lock()
	client := r.client
	r.mu.Unlock()
	if client != nil {
		return client, nil
	}
	if err := r.Connect(); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.client == nil {
		return nil, errors.New("ssh client is not available")
	}
	return r.client, nil
}

func (r *Remoter) ensureSFTP() (*sftp.Client, error) {
	r.mu.Lock()
	if r.sftpClient != nil {
		client := r.sftpClient
		r.mu.Unlock()
		return client, nil
	}
	r.mu.Unlock()

	client, err := r.ensureClient()
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sftpClient != nil {
		return r.sftpClient, nil
	}

	sftpClient, err := client.NewSftp()
	if err != nil {
		return nil, fmt.Errorf("create sftp client failed: %w", err)
	}
	r.sftpClient = sftpClient
	return r.sftpClient, nil
}

func (r *Remoter) renameRemoteFile(sftpClient *sftp.Client, oldPath, newPath string) error {
	if err := sftpClient.PosixRename(oldPath, newPath); err == nil {
		return nil
	}

	info, err := sftpClient.Stat(newPath)
	if err == nil && info.IsDir() {
		return fmt.Errorf("remote path %q is a directory", newPath)
	}
	if err == nil {
		if removeErr := sftpClient.Remove(newPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return removeErr
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	return sftpClient.Rename(oldPath, newPath)
}

func (r *Remoter) uploadSingleFile(localPath, remotePath string, perm os.FileMode) error {
	sftpClient, err := r.ensureSFTP()
	if err != nil {
		return err
	}

	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local file %q failed: %w", localPath, err)
	}
	defer localFile.Close()

	if err := sftpClient.MkdirAll(path.Dir(remotePath)); err != nil {
		return fmt.Errorf("create remote parent dir for %q failed: %w", remotePath, err)
	}

	remoteFile, err := sftpClient.OpenFile(remotePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("open remote file %q failed: %w", remotePath, err)
	}
	defer remoteFile.Close()

	if _, err := io.Copy(remoteFile, localFile); err != nil {
		return fmt.Errorf("upload %q to %q failed: %w", localPath, remotePath, err)
	}
	if err := remoteFile.Chmod(normalizePerm(perm)); err != nil {
		return fmt.Errorf("chmod remote file %q failed: %w", remotePath, err)
	}
	return nil
}

func cleanRemotePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "."
	}
	p = strings.ReplaceAll(p, "\\", "/")
	return path.Clean(p)
}

func joinRemotePath(elem ...string) string {
	parts := make([]string, 0, len(elem))
	for _, item := range elem {
		if strings.TrimSpace(item) == "" {
			continue
		}
		parts = append(parts, strings.ReplaceAll(item, "\\", "/"))
	}
	if len(parts) == 0 {
		return "."
	}
	return path.Clean(path.Join(parts...))
}

func normalizePerm(perm os.FileMode) os.FileMode {
	if perm == 0 {
		return 0o644
	}
	return perm.Perm()
}
