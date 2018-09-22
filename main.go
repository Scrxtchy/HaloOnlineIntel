package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"github.com/gorilla/websocket"
	"github.com/BurntSushi/toml"
	"strings"
	"time"
	"regexp"
	"fmt"
	"log"
)

type tomlConfig struct {
	Servers 				map[string]Server
	Access 					Access
}

type Server struct {
	IP 						string
	Port 					int
	RconPassword 			string
	RconPort 				int
	oldStats				ServerStats
}

type Access struct {
	Port 					int
	Password				string
	Address 				string			`default="0.0.0.0"`
}

type Chat struct {
	Server					string			`json:"server"`
	Time					string			`json:"timestamp"`
	Name					string			`json:"player"`
	UID						string			`json:"UID"`
	IP						string			`json:"IP"`
	Message					string			`json:"message"`
}

func (this Chat) String() string{
	return fmt.Sprintf("[%s] <%s/%s/%s> %s", this.Time, this.Name, this.UID, this.IP, this.Message)
}

type ServerStats struct {
	Name					string			`json:"name"`
	Port					int				`json:"port"`
	HostPlayer				string			`json:"hostPlayer"`
	SprintEnabled			string			`json:"sprintEnabled"`
	SprintUnlimitedEnabled	string			`json:"sprintUnlimitedEnabled"`
	DualWielding			string			`json:"dualWielding"`
	AssassinationEnabled	string			`json:"assassinationEnabled"`
	VotingEnabled			bool			`json:"votingEnabled"`
	Teams					bool			`json:"teams"`
	Map						string			`json:"map"`
	MapFile					string			`json:"mapFile"`
	Variant					string			`json:"variant"`
	VariantType				string			`json:"variantType"`
	Status					string			`json:"status"`
	NumPlayers				int				`json:"numPlayers"`
	Mods					[]interface{}	`json:"mods"`
	MaxPlayers				int				`json:"maxPlayers"`
	Xnkid					string			`json:"xnkid"`
	Xnaddr					string			`json:"xnaddr"`
	Players					[]Player		`json:"players"`
	isDedicated				bool			`json:"isDedicated"`
	gameVersion				string			`json:"gameVersion"`
	eldewritoVersion 		string			`json:"eldewritoVersion"`
}

type Player struct {
	Server					string			`json:"server"`
	Name					string			`json:"name"`
	ServiceTag				string			`json:"serviceTag"`
	UID						string			`json:"uid"`
//	Team					int				`json:"team"`
//	PrimaryColor			string			`json:"primaryColor"`
//	IsAlive					bool			`json:"isAlive"`
//	Score					int				`json:"score"`
//	Kills					int				`json:"kills"`
//	Assists					int				`json:"assists"`
//	Deaths					int				`json:"deaths"`
//	Betrayals				int				`json:"betrayals"`
//	TimeSpentAlive			int				`json:"timeSpentAlive"`
//	Suicides				int				`json:"suicides"`
//	BestStreak				int				`json:"bestStreak"`
}

func (this Player) String() string{
	return fmt.Sprintf("<[%s] %s / %s>", this.ServiceTag, this.Name, this.UID)
}

var wsClients = []*websocket.Conn{}
var upgrader = websocket.Upgrader{}
var config tomlConfig

var toSend []interface{}

var rconRegex = regexp.MustCompile(`\[(.+)\] <(.+)\/([a-f0-9]+)\/(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})> (.+)`)
var msgKey = rconRegex.SubexpNames()

var dewDialer = &websocket.Dialer{
	Proxy:				http.ProxyFromEnvironment,
	HandshakeTimeout: 	45 * time.Second,
	Subprotocols: 		[]string {"dew-rcon"},
}

func handleMsg(message string, serverName string) *Chat{
	matches:= rconRegex.FindStringSubmatch(message)
	if len(matches) < 1{
		return nil
	}
	m := new(Chat)
	m.Time = matches[1]
	m.Name = matches[2]
	m.UID = matches[3]
	m.IP = matches[4]
	m.Message = matches[5]
	m.Server = serverName
	return m
}

func handleReq(w http.ResponseWriter, r *http.Request){
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil || strings.TrimRight(r.URL.Path,"/") != fmt.Sprintf("/%s", config.Access.Password) {
		log.Println("Invalid connection from:", r.RemoteAddr)
		w.WriteHeader(426) //TODO: Handle Hijack? idk
		return
	}
	log.Println("Connection from:", r.RemoteAddr)
	wsClients = append(wsClients, conn)
}

func wsSendMessage(m interface{}) {
	if wsClients != nil{
		for i, client := range wsClients{
			if client.WriteJSON(m) != nil{
				wsClients = append(wsClients[:i], wsClients[i+1:]...)
				client.Close()
			}
		}
	}
}

func wsLoop() {
	for {
		if len(toSend) != 0 {
			wsSendMessage(toSend[0])
			toSend = append(toSend[:0], toSend[1:]...)
		}	
	}
}

func readStats(server *Server, url string, serverName string){
	req, err := http.Get(url)
	if err != nil {
		log.Fatal("req:", err)
	} else{
		var stats ServerStats

		json.NewDecoder(req.Body).Decode(&stats)
		req.Body.Close()

		for _, element := range stats.Players{
			if !contains(server.oldStats.Players, element){
				log.Print(fmt.Sprintf("%s New Player: %s", serverName, element))
				element.Server = serverName
				toSend = append(toSend, element)
			}
		}
		server.oldStats = stats
	}
}

func contains(s []Player, e Player) bool {
	if e.UID == "0000000000000000" {return true}
    for _, a := range s {
        if a.UID == e.UID {
            return true
        }
    }
    return false
}

func connect(serverName string, server Server){
	
	rconURL := url.URL{Scheme: "ws", Host: fmt.Sprintf("%s:%d", server.IP, server.RconPort)}
	serverURL := url.URL{Scheme: "http", Host: fmt.Sprintf("%s:%d", server.IP, server.Port)}
	rconClient, _, err := dewDialer.Dial(rconURL.String(), nil)
	if err != nil{
		log.Fatal("Dial:", err)
	}
	
	if rconClient.WriteMessage(1, []byte(server.RconPassword)) != nil{
		log.Fatal("Password:",err)
	}

	defer rconClient.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, message, err := rconClient.ReadMessage()
			if err != nil {
				log.Println(serverName, err)
				return
			}
			m := handleMsg(string(message[:]), serverName)
			if m != nil {
				log.Println(serverName, m)
				toSend = append(toSend, m)
			}
		}
	}()


	func() {
		for range time.Tick(time.Second *1){
			go readStats(&server, serverURL.String(), serverName)
		}
	}()
}

func main() {
	
	if _, err := toml.DecodeFile("config.toml", &config); err != nil{
		log.Fatal("Config:", err)
	}

	http.HandleFunc("/", handleReq)
	func() {
		for serverName, server := range config.Servers{
			log.Println("Connecting to:", serverName)
			go connect(serverName, server)
		}
	}()

	go wsLoop()
	func() {
		http.ListenAndServe(fmt.Sprintf("%s:%d", config.Access.Address, config.Access.Port), nil)
	}()
}