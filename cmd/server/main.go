package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	qrcode "github.com/skip2/go-qrcode"
)

type Player struct {
	ID       string `json:"player_id"`
	Nickname string `json:"nickname"`
	Score    int    `json:"score"`
}

type Question struct {
	ID      string   `json:"id"`
	Text    string   `json:"text"`
	Options []string `json:"options"`
	Answer  int      `json:"answer"`
}

type AnswerRequest struct {
	PlayerID string `json:"player_id"`
	Answer   int    `json:"answer"`
}

var (
	players = make(map[string]Player)
	mu      sync.Mutex

	currentQuestion   *Question
	questionStartTime int64
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "QuizHub server is running")
}

func joinHandler(w http.ResponseWriter, r *http.Request) {

	type JoinRequest struct {
		Nickname string `json:"nickname"`
	}

	var req JoinRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil || req.Nickname == "" {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	playerID := uuid.New().String()

	player := Player{
		ID:       playerID,
		Nickname: req.Nickname,
		Score:    0,
	}

	mu.Lock()
	players[playerID] = player
	mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(player)
}

func playersHandler(w http.ResponseWriter, r *http.Request) {

	mu.Lock()
	defer mu.Unlock()

	playerList := make([]Player, 0, len(players))

	for _, p := range players {
		playerList = append(playerList, p)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(playerList)
}

func startQuestionHandler(w http.ResponseWriter, r *http.Request) {

	q := Question{
		ID:   "q1",
		Text: "What is 2 + 2?",
		Options: []string{
			"3",
			"4",
			"5",
			"6",
		},
		Answer: 1,
	}

	currentQuestion = &q
	questionStartTime = time.Now().UnixMilli()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(q)
}

func answerHandler(w http.ResponseWriter, r *http.Request) {

	var req AnswerRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if currentQuestion == nil {
		http.Error(w, "no active question", http.StatusBadRequest)
		return
	}

	responseTime := time.Now().UnixMilli() - questionStartTime

	score := 0
	correct := false

	if req.Answer == currentQuestion.Answer {
		correct = true
		score = 10000 - int(responseTime)

		if score < 0 {
			score = 0
		}
	}

	mu.Lock()

	player := players[req.PlayerID]
	player.Score += score
	players[req.PlayerID] = player

	mu.Unlock()

	type AnswerResponse struct {
		Correct bool `json:"correct"`
		Score   int  `json:"score"`
	}

	resp := AnswerResponse{
		Correct: correct,
		Score:   score,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func leaderboardHandler(w http.ResponseWriter, r *http.Request) {

	mu.Lock()
	defer mu.Unlock()

	playerList := make([]Player, 0, len(players))

	for _, p := range players {
		playerList = append(playerList, p)
	}

	sort.Slice(playerList, func(i, j int) bool {
		return playerList[i].Score > playerList[j].Score
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(playerList)
}

func generateQRCode() {

	url := "http://localhost:8080"

	err := qrcode.WriteFile(url, qrcode.Medium, 256, "join-quiz.png")
	if err != nil {
		fmt.Println("Failed to generate QR code:", err)
		return
	}

	fmt.Println("QR code generated: join-quiz.png")
}

func main() {
  http.Handle("/", http.FileServer(http.Dir("./web/static")))
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/join", joinHandler)
	http.HandleFunc("/players", playersHandler)
	http.HandleFunc("/start-question", startQuestionHandler)
	http.HandleFunc("/answer", answerHandler)
	http.HandleFunc("/leaderboard", leaderboardHandler)

	generateQRCode()

	port := "8080"

	if os.Getenv("PORT") != "" {
		port = os.Getenv("PORT")
	}

	fmt.Println("Starting QuizHub server on :" + port)

	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatal(err)
	}
}
