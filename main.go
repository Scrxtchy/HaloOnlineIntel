package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"github.com/gorilla/websocket"
	"flag"
	"fmt"
	"log"
	"time"
)

type Message struct {				
	Rcon					string			`json:"rcon"`
	Stats					ServerStats		`json:"serverStats"`
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
	Players					[]struct {
		Name			string	`json:"name"`
		ServiceTag		string	`json:"serviceTag"`
		Team			int		`json:"team"`
		UID				string	`json:"uid"`
		PrimaryColor	string	`json:"primaryColor"`
		IsAlive			bool	`json:"isAlive"`
		Score			int		`json:"score"`
		Kills			int		`json:"kills"`
		Assists			int		`json:"assists"`
		Deaths			int		`json:"deaths"`
		Betrayals		int		`json:"betrayals"`
		TimeSpentAlive	int		`json:"timeSpentAlive"`
		Suicides		int		`json:"suicides"`
		BestStreak		int		`json:"bestStreak"`
	} `json:"players"`
	isDedicated			bool	`json:"isDedicated"`
	gameVersion			string	`json:"gameVersion"`
	eldewritoVersion 	string	`json:"eldewritoVersion"`
}

var wsClients = []*websocket.Conn{}
var upgrader = websocket.Upgrader{}
var rconPort, serverPort, socketPort int
var rconPass, serverAddress string

var dewDialer = &websocket.Dialer{
	Proxy:				http.ProxyFromEnvironment,
	HandshakeTimeout: 	45 * time.Second,
	Subprotocols: 		[]string {"dew-rcon"},
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

func readStats(url string){
	resp, err := http.Get(url)
	if err != nil {
		log.Fatal("req:", err)
	}
	var stats ServerStats

	json.NewDecoder(resp.Body).Decode(&stats)
	m := new(Message)
	m.Stats = stats
	wsSendMessage(m)


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
			log.Printf("recv: %s", message)
			m := new(Message)
			m.Rcon = string(message[:])
			wsSendMessage(m)
		}
	}()

	func() {
		for range time.Tick(time.Second *5){
			go readStats(serverURL.String())
		}
	}()


}