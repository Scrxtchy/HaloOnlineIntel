package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"github.com/gorilla/websocket"
	"time"
	"flag"
	"regexp"
	"fmt"
	"log"
)

type Message struct {
	Time					string			`json:"timestamp"`
	Name					string			`json:"player"`
	UID						string			`json:"UID"`
	IP						string			`json:"IP"`
	Message					string			`json:"message"`
}

func (this Message) String() string{
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
	Name			string	`json:"name"`
	ServiceTag		string	`json:"serviceTag"`
//	Team			int		`json:"team"`
	UID				string	`json:"uid"`
//	PrimaryColor	string	`json:"primaryColor"`
//	IsAlive			bool	`json:"isAlive"`
//	Score			int		`json:"score"`
//	Kills			int		`json:"kills"`
//	Assists			int		`json:"assists"`
//	Deaths			int		`json:"deaths"`
//	Betrayals		int		`json:"betrayals"`
//	TimeSpentAlive	int		`json:"timeSpentAlive"`
//	Suicides		int		`json:"suicides"`
//	BestStreak		int		`json:"bestStreak"`
}

func (this Player) String() string{
	return fmt.Sprintf("<[%s] %s / %s>", this.ServiceTag, this.Name, this.UID)
}

var wsClients = []*websocket.Conn{}
var upgrader = websocket.Upgrader{}
var rconPort, serverPort, socketPort int
var rconPass, serverAddress string
var oldStats ServerStats

var rconRegex = regexp.MustCompile(`\[(.+)\] <(.+)\/([a-f0-9]+)\/(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})> (.+)`)
var msgKey = rconRegex.SubexpNames()

var dewDialer = &websocket.Dialer{
	Proxy:				http.ProxyFromEnvironment,
	HandshakeTimeout: 	45 * time.Second,
	Subprotocols: 		[]string {"dew-rcon"},
}

func handleMsg(message string) *Message{
	matches:= rconRegex.FindStringSubmatch(message)
	if len(matches) < 1{
		return nil
	}
	m := new(Message)
	m.Time = matches[1]
	m.Name = matches[2]
	m.UID = matches[3]
	m.IP = matches[4]
	m.Message = matches[5]
	return m
}

func handleReq(w http.ResponseWriter, r *http.Request){
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil{
		w.WriteHeader(426)
		return
	}
	wsClients = append(wsClients, conn)
}

func wsSendMessage(m *Message){
	if wsClients != nil{
		for i, client := range wsClients{
			if client.WriteJSON(m) != nil{
				wsClients = append(wsClients[:i], wsClients[i+1:]...)
				client.Close()
			}
		}
	}
}

func wsSendPlayer(p Player){
	if wsClients != nil{
		for i, client := range wsClients{
			if client.WriteJSON(p) != nil{
				wsClients = append(wsClients[:i], wsClients[i+1:]...)
				client.Close()
			}
		}
	}
}

func readStats(url string){
	resp, err := http.Get(url)
	if err != nil {
		log.Fatal("req:", err)
	}
	var stats ServerStats

	json.NewDecoder(resp.Body).Decode(&stats)

	for _, element := range stats.Players{
		if !contains(oldStats.Players, element){
			log.Print(fmt.Sprintf("New Player: %s", element))
			go wsSendPlayer(element)
		}
	}
	oldStats = stats
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

func main() {
	flag.IntVar(&rconPort,"rPort", 11776, "Rcon Port")
	flag.StringVar(&rconPass,"rPass", "", "Rcon Password")
	flag.StringVar(&serverAddress,"sIP", "127.0.0.1", "Server Address")
	flag.IntVar(&serverPort,"sPort", 11775, "Server Port")
	flag.IntVar(&socketPort,"wsPort", 5000, "Local Socket Port")
	flag.Parse()

	http.HandleFunc("/", handleReq)
	go func() {
		http.ListenAndServe(fmt.Sprintf(":%d", socketPort), nil)
	}()

	rconURL := url.URL{Scheme: "ws", Host: fmt.Sprintf("%s:%d", serverAddress, rconPort)}
	serverURL := url.URL{Scheme: "http", Host: fmt.Sprintf("%s:%d", serverAddress, serverPort)}
	rconClient, _, err := dewDialer.Dial(rconURL.String(), nil)
	if err != nil{
		log.Fatal("Dial:", err)
	}
	if rconClient.WriteMessage(1, []byte(rconPass)) != nil{
		log.Fatal(err)
	}

	defer rconClient.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, message, err := rconClient.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			m := handleMsg(string(message[:]))
			if m != nil {
				log.Println("recv:", m)
				go wsSendMessage(m)
			}
		}
	}()

	func() {
		for range time.Tick(time.Second *5){
			go readStats(serverURL.String())
		}
	}()


}