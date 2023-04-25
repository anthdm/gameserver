package main

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"

	"github.com/anthdm/gameserver/types"
	"github.com/anthdm/hollywood/actor"
	"github.com/gorilla/websocket"
)

type PlayerSession struct {
	sessionID int
	clientID  int
	username  string
	inLobby   bool
	conn      *websocket.Conn
	ctx       *actor.Context
	serverPID *actor.PID
}

func newPlayerSession(serverPID *actor.PID, sid int, conn *websocket.Conn) actor.Producer {
	return func() actor.Receiver {
		return &PlayerSession{
			conn:      conn,
			sessionID: sid,
			serverPID: serverPID,
		}
	}
}

func (s *PlayerSession) Receive(c *actor.Context) {
	switch msg := c.Message().(type) {
	case actor.Started:
		s.ctx = c
		go s.readLoop()
	case *types.PlayerState:
		s.sendPlayerState(msg)
	default:
		fmt.Println("recv", msg)
	}
}

func (s *PlayerSession) sendPlayerState(state *types.PlayerState) {
	b, err := json.Marshal(state)
	if err != nil {
		panic(err)
	}
	msg := types.WSMessage{
		Type: "state",
		Data: b,
	}
	if err := s.conn.WriteJSON(msg); err != nil {
		panic(err)
	}
}

func (s *PlayerSession) readLoop() {
	var msg types.WSMessage
	for {
		if err := s.conn.ReadJSON(&msg); err != nil {
			fmt.Println("read error", err)
			return
		}
		go s.handleMessage(msg)
	}
}

func (s *PlayerSession) handleMessage(msg types.WSMessage) {
	switch msg.Type {
	case "login":
		var loginMsg types.Login
		if err := json.Unmarshal(msg.Data, &loginMsg); err != nil {
			panic(err)
		}
		s.clientID = loginMsg.ClientID
		s.username = loginMsg.Username
	case "playerState":
		var ps types.PlayerState
		if err := json.Unmarshal(msg.Data, &ps); err != nil {
			panic(err)
		}
		ps.SessionID = s.sessionID
		if s.ctx != nil {
			s.ctx.Send(s.serverPID, &ps)
		}
	}
}

type GameServer struct {
	ctx      *actor.Context
	sessions map[int]*actor.PID
}

func newGameServer() actor.Receiver {
	return &GameServer{
		sessions: make(map[int]*actor.PID),
	}
}

func (s *GameServer) Receive(c *actor.Context) {
	switch msg := c.Message().(type) {
	case *types.PlayerState:
		s.bcast(c.Sender(), msg)
	case actor.Started:
		s.startHTTP()
		s.ctx = c
		_ = msg
	default:
		fmt.Println("recv", msg)
	}
}

func (s *GameServer) bcast(from *actor.PID, state *types.PlayerState) {
	for _, pid := range s.sessions {
		if !pid.Equals(from) {
			s.ctx.Send(pid, state)
		}
	}
}

func (s *GameServer) startHTTP() {
	fmt.Println("starting HTTP server on port 40000")
	go func() {
		http.HandleFunc("/ws", s.handleWS)
		http.ListenAndServe(":40000", nil)
	}()
}

// handles the upgrade of the websocket
func (s *GameServer) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Upgrade(w, r, nil, 1024, 1024)
	if err != nil {
		fmt.Println("ws upgrade err:", err)
		return
	}
	fmt.Println("new client trying connect")
	sid := rand.Intn(math.MaxInt)
	pid := s.ctx.SpawnChild(newPlayerSession(s.ctx.PID(), sid, conn), fmt.Sprintf("session_%d", sid))
	s.sessions[sid] = pid
}

func main() {
	e := actor.NewEngine()
	e.Spawn(newGameServer, "server")
	select {}
}
