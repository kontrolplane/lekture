package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	bm "github.com/charmbracelet/wish/bubbletea"
	"github.com/kontrolplane/lekture/internal/deck"
	"github.com/kontrolplane/lekture/internal/model"
	"github.com/muesli/termenv"
)

const (
	idleTimeout = 10 * time.Minute
	maxTimeout  = 4 * time.Hour
)

// hostKeyPath returns a stable location for the SSH host key. A path relative
// to the working directory would mint a new host identity per launch
// directory, so returning clients would see the host key change, and would
// scatter private keys through unrelated project trees.
func hostKeyPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(".ssh", "lekture_ed25519")
	}
	return filepath.Join(dir, "lekture", "lekture_ed25519")
}

// sessions tracks the connected clients so a reloaded deck can be pushed to
// all of them.
type sessions struct {
	mu       sync.Mutex
	programs map[*tea.Program]struct{}
}

func newSessions() *sessions {
	return &sessions{programs: make(map[*tea.Program]struct{})}
}

func (s *sessions) add(p *tea.Program) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.programs[p] = struct{}{}
}

func (s *sessions) remove(p *tea.Program) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.programs, p)
}

func (s *sessions) broadcast(msg tea.Msg) {
	s.mu.Lock()
	programs := make([]*tea.Program, 0, len(s.programs))
	for p := range s.programs {
		programs = append(programs, p)
	}
	s.mu.Unlock()

	for _, p := range programs {
		p.Send(msg)
	}
}

// current holds the deck being served, so reloads are visible to clients that
// connect later as well as to those already attached.
type current struct {
	mu sync.RWMutex
	d  *deck.Deck
}

func (c *current) get() *deck.Deck {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.d
}

func (c *current) set(d *deck.Deck) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.d = d
}

// Serve starts an SSH server that presents the deck to connecting clients.
//
// Connections are unauthenticated, so code execution is disabled unless
// allowExec is set: without that gate any client that can reach the port could
// run the deck's code blocks on this machine.
func Serve(d *deck.Deck, host string, port int, allowExec bool) error {
	keyPath := hostKeyPath()
	live := &current{d: d}
	conns := newSessions()

	handler := func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
		pty, _, ok := s.Pty()
		if !ok {
			return nil, nil
		}
		cur := live.get()
		mdl := model.New(cur.Slides, cur.BaseDir, cur.Meta).
			WithExec(allowExec).
			WithSize(pty.Window.Width, pty.Window.Height)
		return mdl, []tea.ProgramOption{tea.WithAltScreen()}
	}

	srv, err := wish.NewServer(
		wish.WithAddress(fmt.Sprintf("%s:%d", host, port)),
		wish.WithHostKeyPath(keyPath),
		wish.WithIdleTimeout(idleTimeout),
		wish.WithMaxTimeout(maxTimeout),
		wish.WithMiddleware(
			bm.MiddlewareWithProgramHandler(func(s ssh.Session) *tea.Program {
				m, opts := handler(s)
				if m == nil {
					return nil
				}
				p := tea.NewProgram(m, append(opts, tea.WithOutput(s), tea.WithInput(s))...)
				conns.add(p)
				go func() {
					<-s.Context().Done()
					conns.remove(p)
				}()
				return p
			}, termenv.ANSI256),
		),
	)
	if err != nil {
		return fmt.Errorf("creating server: %w", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Serve mode watches the deck too, so edits reach viewers without
	// restarting the server and dropping every connection.
	if d.Path != "" {
		go deck.Watch(ctx, d.Path, d.Base(),
			func(next *deck.Deck) {
				if next.Empty() {
					log.Printf("reload skipped: %v", next.EmptyError())
					return
				}
				live.set(next)
				conns.broadcast(model.FileChangedMsg{Slides: next.Slides, Meta: next.Meta})
				log.Printf("reloaded %s (%d slides)", next.Name(), len(next.Slides))
			},
			func(err error) { log.Printf("live reload disabled: %v", err) },
		)
	}

	// Bind before announcing, so a failure to listen does not print
	// connection instructions for a server that never started.
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", srv.Addr, err)
	}

	log.Printf("Starting SSH server on %s", ln.Addr())
	log.Printf("Connect with: ssh %s -p %d", host, port)
	log.Printf("Host key: %s", keyPath)
	if allowExec {
		log.Printf("WARNING: --allow-exec is set; any client that can reach this port can run this deck's code blocks on this machine")
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			return fmt.Errorf("server failed: %w", err)
		}
		return nil
	case sig := <-done:
		log.Printf("Received %v, shutting down...", sig)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	// A deadline here means clients were still connected; that is an expected
	// outcome of Ctrl-C mid-presentation, not a failure worth a non-zero exit.
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}
