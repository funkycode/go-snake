package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"math/rand"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	wishtea "github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
)

const (
	sshHost = "10.0.0.1"
	sshPort = "22255"
)

func sshHandler(s ssh.Session) (tea.Model, []tea.ProgramOption) {
	pty, _, _ := s.Pty()
	game := NewGame()
	game.windowWidth = pty.Window.Width
	game.windowHeight = pty.Window.Height
	game.sessionStyle = wishtea.MakeRenderer(s).NewStyle()
	return game, []tea.ProgramOption{
		tea.WithAltScreen(),
	}
}

type tickMsg time.Time

func refresh() tea.Cmd {
	return tea.Tick(220*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// FIXME: seems to not to work over ssh, need to use bubbletea.MakeRender
var (
	boardStyle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, true, true, true)
)

type position struct {
	x    int
	y    int
	next *position
}

type apple struct {
	x int
	y int
}

type direction int

const (
	still direction = iota
	up
	down
	left
	right
)

type snake struct {
	head      *position
	direction direction
}

func NewSnake(x, y int) *snake {
	return &snake{
		head: &position{
			x: x,
			y: y,
		},
	}
}

type game struct {
	sessionStyle              lipgloss.Style
	paused                    bool
	windowHeight, windowWidth int
	gameover                  bool
	width, height             int
	boardView                 viewport.Model
	boardContent              [][]string
	snake                     *snake
	apple                     *apple
}

func (g *game) resetBoard() {
	boardContent := make([][]string, g.height)
	for i := range boardContent {
		row := make([]string, g.width)
		for j := range row {
			row[j] = " "
		}
		boardContent[i] = row
	}
	g.boardContent = boardContent
}

func (g *game) setSnake() {
	pos := g.snake.head
	for pos != nil {
		colourNum := rand.Intn(12)
		colour := lipgloss.Color(strconv.Itoa(colourNum))
		bg := g.sessionStyle.GetBackground()
		if bg == colour {
			colour = lipgloss.Color("255")
		}
		style := g.sessionStyle.Foreground(colour)
		row := g.boardContent[pos.x]
		row[pos.y] = style.Render("■")
		g.boardContent[pos.x] = row
		pos = pos.next
	}
}

func (g *game) setApple() {
	row := g.boardContent[g.apple.x]
	row[g.apple.y] = "@"
	g.boardContent[g.apple.x] = row
}

func (g *game) generateApple() {
	randX := rand.Intn(g.height)
	randY := rand.Intn(g.width)
	pos := g.snake.head
	for pos != nil {
		if pos.x == randX && pos.y == randY {
			g.generateApple()
			return
		}
		pos = pos.next
	}
	g.apple = &apple{
		x: randX,
		y: randY,
	}
}

func (g *game) moveSnake(direction direction) {
	if g.paused || g.gameover {
		return
	}
	newHeadX := g.snake.head.x
	newHeadY := g.snake.head.y

	switch direction {
	case up:
		newHeadX--
	case down:
		newHeadX++
	case left:
		newHeadY--
	case right:
		newHeadY++
	}
	if newHeadX < 0 || newHeadX >= g.height || newHeadY < 0 || newHeadY >= g.width {
		g.gameover = true
		return
	}

	newHead := &position{
		x: newHeadX,
		y: newHeadY,
	}

	var eatApple bool

	if g.apple.x == newHead.x && g.apple.y == newHead.y {
		eatApple = true
	}

	prevPos := g.snake.head
	g.snake.direction = direction

	// if we only had length of one
	if g.snake.head.next == nil && eatApple {
		g.snake.head = newHead
		g.snake.head.next = prevPos
		g.generateApple()
		return
	}

	newPos := newHead
	for prevPos != nil {
		// last one should be dropped if we do not eat the apple
		if prevPos.next == nil && !eatApple {
			newPos.next = nil
			break
		}
		if newHead.x == prevPos.x && newHead.y == prevPos.y {
			// hit ourself
			g.gameover = true
			return
		}
		newPos.next = prevPos
		newPos = newPos.next
		prevPos = prevPos.next

	}
	g.snake.head = newHead
	g.snake.direction = direction
	if eatApple {
		g.generateApple()
	}

}

func NewGame() *game {
	width := 40
	height := 20
	snake := NewSnake(height/2-1, width/2-1)
	board := viewport.New(width, height)
	return &game{
		width:     width,
		height:    height,
		snake:     snake,
		boardView: board,
		apple:     &apple{},
	}
}

func (g *game) Init() tea.Cmd {
	g.generateApple()
	return refresh()
}

func (g *game) View() string {
	if g.windowWidth < g.width || g.windowHeight < g.height {
		g.paused = true
		return "Window is too small"
	}
	g.paused = false
	if g.gameover {
		return "Game Over"
	}
	g.resetBoard()
	g.setSnake()
	g.setApple()
	b := strings.Builder{}
	for i := range g.boardContent {
		b.WriteString(strings.Join(g.boardContent[i], ""))
		b.WriteString("\n")
	}
	g.boardView.SetContent(b.String())
	view := boardStyle.Render(g.boardView.View())
	return lipgloss.Place(g.windowWidth, g.windowHeight, lipgloss.Center, lipgloss.Center, view)
}

func (g *game) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		g.windowHeight = msg.Height
		g.windowWidth = msg.Width
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyRunes:
			switch msg.String() {
			case "q":
				return g, tea.Quit
			case "h":
				if g.snake.direction != right {
					g.snake.direction = left
				}
			case "j":
				if g.snake.direction != up {
					g.snake.direction = down
				}
			case "k":
				if g.snake.direction != down {
					g.snake.direction = up
				}
			case "l":
				if g.snake.direction != left {
					g.snake.direction = right
				}
			}
		case tea.KeyUp:
			if g.snake.direction != down {
				g.snake.direction = up
			}
		case tea.KeyDown:
			if g.snake.direction != up {
				g.snake.direction = down
			}
		case tea.KeyLeft:
			if g.snake.direction != right {
				g.snake.direction = left
			}
		case tea.KeyRight:
			if g.snake.direction != left {
				g.snake.direction = right
			}
		default:
		}
	case tickMsg:
		g.moveSnake(g.snake.direction)
		return g, refresh()
	default:
	}
	return g, nil
}

func main() {
	serv, err := wish.NewServer(
		wish.WithAddress(net.JoinHostPort(sshHost, sshPort)),
		wish.WithMiddleware(
			wishtea.Middleware(sshHandler),
			activeterm.Middleware(),
			logging.Middleware(),
		),
	)
	if err != nil {
		log.Fatalf("Failed to start: %v", err)
	}
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		if err := serv.ListenAndServe(); err != nil {
			log.Fatalf("Failed to start: %v", err)
		}
	}()
	<-done
}
