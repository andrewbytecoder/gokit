package remoter

import (
	"context"
	"os"
)

// IRemoter defines the remote operation interface, wrapping SSH + SFTP capabilities
// on top of goph (github.com/melbahja/goph).
type IRemoter interface {
	// Connect establishes an SSH connection; must be called before any other method.
	Connect() error
	// Close terminates the SSH connection.
	Close() error

	// RunCmd executes a command on the remote host and returns combined output (stdout + stderr).
	RunCmd(cmd string) ([]byte, error)
	// RunCmds executes multiple commands in a single remote shell context.
	RunCmds(cmds ...string) ([]byte, error)
	// RunCmdContext executes a command with context, supporting timeout and cancellation.
	RunCmdContext(ctx context.Context, cmd string) ([]byte, error)
	// RunCmdsContext executes multiple commands in a single remote shell context with context support.
	RunCmdsContext(ctx context.Context, cmds ...string) ([]byte, error)

	// UploadFile uploads a local file to the remote host via SFTP.
	UploadFile(localPath, remotePath string) error
	// UploadDir recursively syncs a local directory to the remote host.
	UploadDir(localDir, remoteDir string) error

	// DownloadFile downloads a remote file to the local machine via SFTP.
	DownloadFile(remotePath, localPath string) error

	// PathExists checks whether the given path (file or directory) exists on the remote host.
	PathExists(remotePath string) (bool, error)

	// CrateRemoteDir creates a remote directory recursively if needed.
	CrateRemoteDir(remoteDir string) error
	// DeleteFile deletes a file on the remote host.
	DeleteFile(remotePath string) error
	// DeleteDir recursively deletes a directory on the remote host.
	DeleteDir(remotePath string) error

	// WriteFile writes content to a remote file. Creates the file if it does not exist,
	// overwrites it if it does.
	WriteFile(remotePath string, content []byte, perm os.FileMode) error
	// ReadFile reads the content of a remote file.
	ReadFile(remotePath string) ([]byte, error)
}
