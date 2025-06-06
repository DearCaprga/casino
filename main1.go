package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

// Card представляет карту в игре
type Card struct {
	ID      int    `json:"id"`
	Value   string `json:"value"`
	Flipped bool   `json:"flipped"`
	Matched bool   `json:"matched"`
}

// Player представляет игрока
type Player struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	Score        int       `json:"score"`
	Coins        int       `json:"coins"`
	Achievements []string  `json:"achievements"`
	GamesPlayed  int       `json:"games_played"`
	CreatedAt    time.Time `json:"created_at"`
}

// GameState представляет состояние игры
type GameState struct {
	Cards       []Card     `json:"cards"`
	PlayerID    int        `json:"player_id"`
	Started     bool       `json:"started"`
	Finished    bool       `json:"finished"`
	LastFlipped []int      `json:"last_flipped"`
	StartTime   time.Time  `json:"start_time"`
	EndTime     *time.Time `json:"end_time,omitempty"`
	Difficulty  string     `json:"difficulty"` // easy, medium, hard
	TimeLimit   int        `json:"time_limit"` // в секундах
}

// LeaderboardEntry представляет запись в таблице лидеров
type LeaderboardEntry struct {
	PlayerID   int    `json:"player_id"`
	PlayerName string `json:"player_name"`
	Score      int    `json:"score"`
	GamesWon   int    `json:"games_won"`
}

var (
	players      []Player
	games        map[int]GameState
	leaderboard  []LeaderboardEntry
	achievements = []string{
		"Первая игра",
		"5 игр сыграно",
		"10 игр сыграно",
		"Быстрая победа",
		"Сложный уровень",
		"Мастер памяти",
	}
	mutex sync.Mutex
)

func main() {
	// Создание или открытие базы данных
	db, err := newDB()
	if err != nil {
		log.Fatalf("Ошибка при подключении к базе данных: %v", err)
	}
	defer db.Close()
	// Инициализация данных
	games = make(map[int]GameState)
	players = append(players, Player{
		ID:        1,
		Name:      "Игрок 1",
		Score:     0,
		Coins:     1000,
		CreatedAt: time.Now(),
	})

	//создание таблицы с игроками
	createTables(db)

	// Инициализация таблицы лидеров
	updateLeaderboard()

	r := mux.NewRouter()
	registerRoutes(r)

	log.Println("Memory Casino Server is running at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

func registerRoutes(r *mux.Router) {
	// Игроки
	r.HandleFunc("/players", getPlayersHandler).Methods("GET")
	r.HandleFunc("/players", createPlayerHandler).Methods("POST")
	r.HandleFunc("/players/{id}", getPlayerHandler).Methods("GET")
	r.HandleFunc("/players/{id}/achievements", getPlayerAchievementsHandler).Methods("GET")

	// Игра
	r.HandleFunc("/game/{player_id}/start", startGameHandler).Methods("POST")
	r.HandleFunc("/game/{player_id}/flip/{card_id}", flipCardHandler).Methods("POST")
	r.HandleFunc("/game/{player_id}/state", getGameStateHandler).Methods("GET")
	r.HandleFunc("/game/{player_id}/end", endGameHandler).Methods("POST")

	// Таблица лидеров
	r.HandleFunc("/leaderboard", getLeaderboardHandler).Methods("GET")

	// Статистика
	r.HandleFunc("/stats", getGameStatsHandler).Methods("GET")
}

func newDB() (*sql.DB, error) {
	// Подключение к базе данных SQLite
	db, err := sql.Open("sqlite3", "players.db")
	if err != nil {
		return nil, fmt.Errorf("ошибка подключения к базе данных: %w", err)
	}

	// Проверка соединения
	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("ошибка ping базы данных: %w", err)
	}

	log.Println("Успешно подключено к базе данных my_sqlite.db")
	return db, nil
}

func createTables(db *sql.DB) {
	players_bd := `
	CREATE TABLE IF NOT EXISTS players (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name text NOT NULL,
	score INTEGER,
	coins INTEGER,
	GamesPlayed INTEGER);`

	achievements_bd := `
	CREATE TABLE IF NOT EXISTS achievements(
	title TEXT CHECK(status IN ("Первая игра",
		"5 игр сыграно",
		"10 игр сыграно",
		"Быстрая победа",
		"Сложный уровень",
		"Мастер памяти")),
	user_id INTEGER,
	FOREIGN KEY(user_id) REFERENCES players (id));`

	_, err := db.Exec(players_bd)
	if err != nil {
		log.Fatalf("Ошибка создания таблицы users: %v", err)
	}

	_, err = db.Exec(achievements_bd)
	if err != nil {
		log.Fatalf("Ошибка создания таблицы tasks: %v", err)
	}
}

func endGameForPlayer(playerID int, won bool) {
	if game, exists := games[playerID]; exists {
		now := time.Now()
		game.EndTime = &now
		game.Finished = true
		games[playerID] = game

		// Обновляем статистику игрока
		for i := range players {
			if players[i].ID == playerID {
				players[i].GamesPlayed++

				if won {
					// Награда за победу
					players[i].Coins += 1000
					players[i].Score += 500

					// Проверяем достижения
					checkAchievements(&players[i], "game_won")
				}

				// Проверяем другие достижения
				checkAchievements(&players[i], "games_played")
				break
			}
		}
	}
}

func checkAchievements(player *Player, trigger string) {
	mutex.Lock()
	defer mutex.Unlock()

	switch trigger {
	case "games_played":
		if player.GamesPlayed >= 5 && !contains(player.Achievements, "5 игр сыграно") {
			player.Achievements = append(player.Achievements, "5 игр сыграно")
			player.Coins += 200
		}
		if player.GamesPlayed >= 10 && !contains(player.Achievements, "10 игр сыграно") {
			player.Achievements = append(player.Achievements, "10 игр сыграно")
			player.Coins += 500
		}
	case "game_won":
		if !contains(player.Achievements, "Первая игра") {
			player.Achievements = append(player.Achievements, "Первая игра")
			player.Coins += 100
		}
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func createPlayerHandler(w http.ResponseWriter, r *http.Request) {
	db, err := newDB()
	if err != nil {
		http.Error(w, fmt.Sprintf("Ошибка подключения к БД: %v", err),
			http.StatusInternalServerError)
		return
	}
	defer db.Close()

	var player Player
	if err := json.NewDecoder(r.Body).Decode(&player); err != nil {
		http.Error(w, "Некорректный JSON", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Устанавливаем значения по умолчанию
	player.Coins = 1000
	player.GamesPlayed = 0
	player.Score = 0
	var name string
	fmt.Print("Введите имя: ")
	fmt.Scan(&name)
	player.Name = name

	// Вставляем игрока в БД
	query := `
        INSERT INTO players (name, score, coins, GamesPlayed, CreatedAt) 
        VALUES (?, ?, ?, ?, ?)
        RETURNING id
    `
	err = db.QueryRow(
		query,
		player.Name,
		player.Score,
		player.Coins,
		player.GamesPlayed,
	).Scan(&player.ID)

	if err != nil {
		http.Error(w, fmt.Sprintf("Ошибка создания игрока: %v", err),
			http.StatusInternalServerError)
		return
	}

	// Отправляем созданного игрока в ответе
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(player); err != nil {
		http.Error(w, fmt.Sprintf("Ошибка кодирования JSON: %v", err),
			http.StatusInternalServerError)
	}
}

func getPlayerAchievementsHandler(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Некорректный ID", http.StatusBadRequest)
		return
	}

	for _, player := range players {
		if player.ID == id {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"player_id":    player.ID,
				"achievements": player.Achievements,
			})
			return
		}
	}

	http.NotFound(w, r)
}

// Обработчики игры (обновленные)

func startGameHandler(w http.ResponseWriter, r *http.Request) {
	playerIDStr := mux.Vars(r)["player_id"]
	playerID, err := strconv.Atoi(playerIDStr)
	if err != nil {
		http.Error(w, "Некорректный ID игрока", http.StatusBadRequest)
		return
	}

	// Получаем параметр сложности
	difficulty := r.URL.Query().Get("difficulty")
	if difficulty == "" {
		difficulty = "medium"
	}

	// Проверяем, что игрок существует
	var player *Player
	for i, p := range players {
		if p.ID == playerID {
			player = &players[i]
			break
		}
	}

	if player == nil {
		http.Error(w, "Игрок не найден", http.StatusNotFound)
		return
	}

	// Определяем стоимость игры в зависимости от сложности
	var cost, timeLimit int
	switch difficulty {
	case "easy":
		cost = 30
		timeLimit = 300 // 5 минут
	case "medium":
		cost = 50
		timeLimit = 240 // 4 минуты
	case "hard":
		cost = 80
		timeLimit = 180 // 3 минуты
	default:
		http.Error(w, "Некорректный уровень сложности", http.StatusBadRequest)
		return
	}

	// Проверяем, что у игрока достаточно монет для игры
	if player.Coins < cost {
		http.Error(w, "Недостаточно монет для начала игры", http.StatusBadRequest)
		return
	}

	mutex.Lock()
	defer mutex.Unlock()

	// Снимаем ставку
	player.Coins -= cost

	// Создаем новую игру
	game := GameState{
		PlayerID:   playerID,
		Started:    true,
		Finished:   false,
		Cards:      createShuffledDeck(difficulty),
		StartTime:  time.Now(),
		Difficulty: difficulty,
		TimeLimit:  timeLimit,
	}

	games[playerID] = game

	// Проверяем достижение для сложного уровня
	if difficulty == "hard" && !contains(player.Achievements, "Сложный уровень") {
		player.Achievements = append(player.Achievements, "Сложный уровень")
		player.Coins += 300
	}

	json.NewEncoder(w).Encode(game)
}

func flipCardHandler(w http.ResponseWriter, r *http.Request) {
	playerIDStr := mux.Vars(r)["player_id"]
	playerID, err := strconv.Atoi(playerIDStr)
	if err != nil {
		http.Error(w, "Некорректный ID игрока", http.StatusBadRequest)
		return
	}

	cardIDStr := mux.Vars(r)["card_id"]
	cardID, err := strconv.Atoi(cardIDStr)
	if err != nil {
		http.Error(w, "Некорректный ID карты", http.StatusBadRequest)
		return
	}

	mutex.Lock()
	defer mutex.Unlock()

	game, exists := games[playerID]
	if !exists || !game.Started || game.Finished {
		http.Error(w, "Игра не начата или уже завершена", http.StatusBadRequest)
		return
	}

	// Проверяем, не истекло ли время
	if time.Since(game.StartTime).Seconds() > float64(game.TimeLimit) {
		endGameForPlayer(playerID, false)
		http.Error(w, "Время игры истекло", http.StatusBadRequest)
		return
	}

	// Находим карту
	var card *Card
	for i := range game.Cards {
		if game.Cards[i].ID == cardID {
			card = &game.Cards[i]
			break
		}
	}

	if card == nil {
		http.Error(w, "Карта не найдена", http.StatusNotFound)
		return
	}

	// Нельзя переворачивать уже совпавшие или уже перевернутые карты
	if card.Matched || card.Flipped {
		http.Error(w, "Карта уже перевернута или совпала", http.StatusBadRequest)
		return
	}

	// Переворачиваем карту
	card.Flipped = true

	// Добавляем в список последних перевернутых
	game.LastFlipped = append(game.LastFlipped, card.ID)

	// Если перевернуто 2 карты, проверяем на совпадение
	if len(game.LastFlipped) == 2 {
		var firstCard, secondCard *Card
		for i := range game.Cards {
			if game.Cards[i].ID == game.LastFlipped[0] {
				firstCard = &game.Cards[i]
			}
			if game.Cards[i].ID == game.LastFlipped[1] {
				secondCard = &game.Cards[i]
			}
		}

		if firstCard.Value == secondCard.Value {
			// Совпадение!
			firstCard.Matched = true
			secondCard.Matched = true

			// Начисляем очки игроку
			for i := range players {
				if players[i].ID == playerID {
					players[i].Score += 100
					players[i].Coins += 200 // Выигрыш

					// Проверяем достижение "Мастер памяти"
					if !contains(players[i].Achievements, "Мастер памяти") {
						// Проверяем, все ли карты совпали
						allMatched := true
						for _, c := range game.Cards {
							if !c.Matched {
								allMatched = false
								break
							}
						}

						if allMatched {
							players[i].Achievements = append(players[i].Achievements, "Мастер памяти")
							players[i].Coins += 1000
						}
					}
					break
				}
			}
		}

		// Сбрасываем список последних перевернутых
		game.LastFlipped = nil
	}

	// Проверяем, закончена ли игра (все карты совпали)
	allMatched := true
	for _, c := range game.Cards {
		if !c.Matched {
			allMatched = false
			break
		}
	}

	if allMatched {
		// Проверяем, была ли игра завершена быстро (менее чем за 1 минуту)
		if time.Since(game.StartTime).Seconds() < 60 {
			for i := range players {
				if players[i].ID == playerID && !contains(players[i].Achievements, "Быстрая победа") {
					players[i].Achievements = append(players[i].Achievements, "Быстрая победа")
					players[i].Coins += 500
					break
				}
			}
		}

		endGameForPlayer(playerID, true)
	}

	// Обновляем состояние игры
	games[playerID] = game

	json.NewEncoder(w).Encode(game)
}

// Обработчики таблицы лидеров и статистики

func getLeaderboardHandler(w http.ResponseWriter, r *http.Request) {
	updateLeaderboard()
	json.NewEncoder(w).Encode(leaderboard)
}

func updateLeaderboard() {
	var entries []LeaderboardEntry

	for _, player := range players {
		gamesWon := 0
		// В реальном приложении мы бы хранили статистику побед отдельно
		// Здесь для простоты считаем, что каждая завершенная игра - победа
		gamesWon = player.GamesPlayed

		entries = append(entries, LeaderboardEntry{
			PlayerID:   player.ID,
			PlayerName: player.Name,
			Score:      player.Score,
			GamesWon:   gamesWon,
		})
	}

	// Сортируем по убыванию очков
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Score > entries[j].Score
	})

	leaderboard = entries
}

func getGameStatsHandler(w http.ResponseWriter, r *http.Request) {
	stats := map[string]interface{}{
		"total_players":  len(players),
		"total_games":    countTotalGames(),
		"active_games":   countActiveGames(),
		"top_player":     getTopPlayer(),
		"recent_winners": getRecentWinners(5),
	}

	json.NewEncoder(w).Encode(stats)
}

func countTotalGames() int {
	total := 0
	for _, player := range players {
		total += player.GamesPlayed
	}
	return total
}

func countActiveGames() int {
	count := 0
	for _, game := range games {
		if game.Started && !game.Finished {
			count++
		}
	}
	return count
}

func getTopPlayer() *LeaderboardEntry {
	if len(leaderboard) > 0 {
		return &leaderboard[0]
	}
	return nil
}

func getRecentWinners(count int) []LeaderboardEntry {
	if count > len(leaderboard) {
		count = len(leaderboard)
	}
	return leaderboard[:count]
}

// Вспомогательные функции

func createShuffledDeck(difficulty string) []Card {
	var deckSize int
	switch difficulty {
	case "easy":
		deckSize = 8 // 4 пары
	case "medium":
		deckSize = 12 // 6 пар
	case "hard":
		deckSize = 16 // 8 пар
	default:
		deckSize = 12
	}

	// Создаем базовый набор символов
	symbols := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	if deckSize/2 > len(symbols) {
		// Если нужно больше пар, чем у нас символов, дублируем существующие
		for i := 0; len(symbols) < deckSize/2; i++ {
			symbols = append(symbols, symbols[i]+"'")
		}
	}

	// Создаем колоду с парами
	var deck []string
	for i := 0; i < deckSize/2; i++ {
		deck = append(deck, symbols[i], symbols[i])
	}

	// Перемешиваем колоду
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })

	var cards []Card
	for i, value := range deck {
		cards = append(cards, Card{
			ID:      i + 1,
			Value:   value,
			Flipped: false,
			Matched: false,
		})
	}

	return cards
}

func getPlayersHandler(w http.ResponseWriter, r *http.Request) {
	db, err := newDB()
	if err != nil {
		http.Error(w, fmt.Sprintf("Ошибка подключения к базе данных: %v", err),
			http.StatusInternalServerError)
		return
	}
	defer db.Close()

	query := `
        SELECT id, name, score, coins, GamesPlayed 
        FROM players
        ORDER BY score DESC
    `
	rows, err := db.Query(query)
	if err != nil {
		http.Error(w, fmt.Sprintf("Ошибка выполнения запроса: %v", err),
			http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var players []Player
	for rows.Next() {
		var p Player
		err := rows.Scan(&p.ID, &p.Name, &p.Score, &p.Coins, &p.GamesPlayed)
		if err != nil {
			http.Error(w, fmt.Sprintf("Ошибка сканирования данных игрока: %v", err),
				http.StatusInternalServerError)
			return
		}
		players = append(players, p)
	}

	if err = rows.Err(); err != nil {
		http.Error(w, fmt.Sprintf("Ошибка при обработке результатов: %v", err),
			http.StatusInternalServerError)
		return
	}

	//ответ
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(players); err != nil {
		http.Error(w, fmt.Sprintf("Ошибка кодирования JSON: %v", err),
			http.StatusInternalServerError)
	}
}

// getPlayerHandler возвращает информацию об игроке по ID
func getPlayerHandler(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Некорректный ID", http.StatusBadRequest)
		return
	}

	mutex.Lock()
	defer mutex.Unlock()

	for _, player := range players {
		if player.ID == id {
			json.NewEncoder(w).Encode(player)
			return
		}
	}

	http.NotFound(w, r)
}

// getGameStateHandler возвращает текущее состояние игры для игрока
func getGameStateHandler(w http.ResponseWriter, r *http.Request) {
	playerIDStr := mux.Vars(r)["player_id"]
	playerID, err := strconv.Atoi(playerIDStr)
	if err != nil {
		http.Error(w, "Некорректный ID игрока", http.StatusBadRequest)
		return
	}

	mutex.Lock()
	defer mutex.Unlock()

	game, exists := games[playerID]
	if !exists {
		http.NotFound(w, r)
		return
	}

	// Добавляем оставшееся время
	timeLeft := game.TimeLimit - int(time.Since(game.StartTime).Seconds())
	if timeLeft < 0 {
		timeLeft = 0
	}

	response := struct {
		GameState
		TimeLeft int `json:"time_left"`
	}{
		GameState: game,
		TimeLeft:  timeLeft,
	}

	json.NewEncoder(w).Encode(response)
}

// endGameHandler завершает игру досрочно
func endGameHandler(w http.ResponseWriter, r *http.Request) {
	playerIDStr := mux.Vars(r)["player_id"]
	playerID, err := strconv.Atoi(playerIDStr)
	if err != nil {
		http.Error(w, "Некорректный ID игрока", http.StatusBadRequest)
		return
	}

	mutex.Lock()
	defer mutex.Unlock()

	if game, exists := games[playerID]; exists {
		if !game.Finished {
			// Завершаем игру с флагом "не выиграна"
			endGameForPlayer(playerID, false)
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	http.NotFound(w, r)
}
