package main

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed web/*
var webFiles embed.FS

type app struct {
	db        *sql.DB
	client    *http.Client
	userAgent string
}

type food struct {
	ID           int64              `json:"id"`
	Name         string             `json:"name"`
	Brand        string             `json:"brand"`
	Barcode      string             `json:"barcode"`
	ServingSize  float64            `json:"servingSize"`
	ServingUnit  string             `json:"servingUnit"`
	Calories     float64            `json:"calories"`
	Protein      float64            `json:"protein"`
	Carbs        float64            `json:"carbs"`
	Fat          float64            `json:"fat"`
	Fiber        float64            `json:"fiber"`
	Sugar        float64            `json:"sugar"`
	SaturatedFat float64            `json:"saturatedFat"`
	Salt         float64            `json:"salt"`
	Micros       map[string]float64 `json:"micros"`
	Source       string             `json:"source"`
	ImageURL     string             `json:"imageUrl"`
	NeedsServing bool               `json:"needsServingSetup,omitempty"`
}

type entry struct {
	ID         int64   `json:"id"`
	FoodID     *int64  `json:"foodId"`
	Date       string  `json:"date"`
	Meal       string  `json:"meal"`
	Amount     float64 `json:"amount"`
	Quantity   float64 `json:"quantity"`
	UnitAmount float64 `json:"unitAmount"`
	IsCustom   bool    `json:"isCustom"`
	Food       food    `json:"food"`
	CreatedAt  string  `json:"createdAt"`
}

type goals struct {
	Calories float64 `json:"calories"`
	Protein  float64 `json:"protein"`
	Carbs    float64 `json:"carbs"`
	Fat      float64 `json:"fat"`
	Fiber    float64 `json:"fiber"`
}

type dailyStats struct {
	Date             string   `json:"date"`
	Weight           *float64 `json:"weight"`
	CaloriesBurned   *float64 `json:"caloriesBurned"`
	IntakeIncomplete bool     `json:"intakeIncomplete"`
}

type historyPoint struct {
	Date             string   `json:"date"`
	CaloriesIn       *float64 `json:"caloriesIn"`
	Weight           *float64 `json:"weight"`
	CaloriesBurned   *float64 `json:"caloriesBurned"`
	IntakeIncomplete bool     `json:"intakeIncomplete"`
}

type offNutriments struct {
	Energy       float64 `json:"energy_100g"`
	EnergyKcal   float64 `json:"energy-kcal_100g"`
	Protein      float64 `json:"proteins_100g"`
	Carbs        float64 `json:"carbohydrates_100g"`
	Fat          float64 `json:"fat_100g"`
	Fiber        float64 `json:"fiber_100g"`
	Sugar        float64 `json:"sugars_100g"`
	SaturatedFat float64 `json:"saturated-fat_100g"`
	Salt         float64 `json:"salt_100g"`
	Sodium       float64 `json:"sodium_100g"`
	Calcium      float64 `json:"calcium_100g"`
	Iron         float64 `json:"iron_100g"`
	Magnesium    float64 `json:"magnesium_100g"`
	Potassium    float64 `json:"potassium_100g"`
	Zinc         float64 `json:"zinc_100g"`
	VitaminC     float64 `json:"vitamin-c_100g"`
	VitaminB12   float64 `json:"vitamin-b12_100g"`
	VitaminD     float64 `json:"vitamin-d_100g"`
}

func main() {
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")

	dbPath := env("DATABASE_PATH", "./data/mampftracker.db")
	if err := os.MkdirAll(path.Dir(dbPath), 0o755); err != nil {
		log.Fatal(err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := migrate(db); err != nil {
		log.Fatal(err)
	}

	a := &app{
		db:        db,
		client:    &http.Client{Timeout: 10 * time.Second},
		userAgent: env("OPENFOODFACTS_USER_AGENT", "MampfTracker/0.1 (self-hosted; contact@example.invalid)"),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", a.health)
	mux.HandleFunc("GET /api/foods", a.listFoods)
	mux.HandleFunc("POST /api/foods", a.createFood)
	mux.HandleFunc("GET /api/foods/barcode/{barcode}", a.barcode)
	mux.HandleFunc("PUT /api/foods/{id}", a.updateFood)
	mux.HandleFunc("PUT /api/foods/{id}/serving", a.updateFoodServing)
	mux.HandleFunc("GET /api/entries", a.listEntries)
	mux.HandleFunc("POST /api/entries", a.createEntry)
	mux.HandleFunc("PUT /api/entries/{id}", a.updateEntry)
	mux.HandleFunc("DELETE /api/entries/{id}", a.deleteEntry)
	mux.HandleFunc("GET /api/goals", a.getGoals)
	mux.HandleFunc("PUT /api/goals", a.putGoals)
	mux.HandleFunc("GET /api/daily-stats", a.getDailyStats)
	mux.HandleFunc("PUT /api/daily-stats", a.putDailyStats)
	mux.HandleFunc("GET /api/history", a.getHistory)
	mux.Handle("/", spaHandler())

	server := &http.Server{
		Addr:              ":" + env("PORT", "8080"),
		Handler:           logging(basicAuth(mux)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("MampfTracker hört auf %s", server.Addr)
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS foods (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			brand TEXT NOT NULL DEFAULT '',
			barcode TEXT NOT NULL DEFAULT '',
			serving_size REAL NOT NULL DEFAULT 100,
			serving_unit TEXT NOT NULL DEFAULT 'g',
			calories REAL NOT NULL DEFAULT 0,
			protein REAL NOT NULL DEFAULT 0,
			carbs REAL NOT NULL DEFAULT 0,
			fat REAL NOT NULL DEFAULT 0,
			fiber REAL NOT NULL DEFAULT 0,
			sugar REAL NOT NULL DEFAULT 0,
			saturated_fat REAL NOT NULL DEFAULT 0,
			salt REAL NOT NULL DEFAULT 0,
			micros TEXT NOT NULL DEFAULT '{}',
			source TEXT NOT NULL DEFAULT 'manual',
			image_url TEXT NOT NULL DEFAULT '',
			serving_configured INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE UNIQUE INDEX IF NOT EXISTS foods_barcode_unique
			ON foods(barcode) WHERE barcode <> '';
		CREATE INDEX IF NOT EXISTS foods_name ON foods(name);

		CREATE TABLE IF NOT EXISTS entries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			food_id INTEGER REFERENCES foods(id) ON DELETE RESTRICT,
			entry_date TEXT NOT NULL,
			meal TEXT NOT NULL,
			amount REAL NOT NULL CHECK(amount > 0),
			quantity REAL NOT NULL DEFAULT 1,
			unit_amount REAL,
			custom_name TEXT NOT NULL DEFAULT '',
			custom_calories REAL NOT NULL DEFAULT 0,
			custom_protein REAL NOT NULL DEFAULT 0,
			custom_carbs REAL NOT NULL DEFAULT 0,
			custom_fat REAL NOT NULL DEFAULT 0,
			custom_fiber REAL NOT NULL DEFAULT 0,
			custom_sugar REAL NOT NULL DEFAULT 0,
			custom_saturated_fat REAL NOT NULL DEFAULT 0,
			custom_salt REAL NOT NULL DEFAULT 0,
			custom_micros TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS entries_date ON entries(entry_date);

		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS daily_stats (
			entry_date TEXT PRIMARY KEY,
			weight REAL,
			calories_burned REAL,
			intake_incomplete INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		INSERT OR IGNORE INTO settings(key, value)
		VALUES ('goals', '{"calories":2200,"protein":140,"carbs":250,"fat":70,"fiber":30}');
	`)
	if err != nil {
		return err
	}
	// Migration for databases created before serving_configured existed.
	_, err = db.Exec(`ALTER TABLE foods ADD COLUMN serving_configured INTEGER NOT NULL DEFAULT 1`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	for _, migration := range []string{
		`ALTER TABLE entries ADD COLUMN quantity REAL NOT NULL DEFAULT 1`,
		`ALTER TABLE entries ADD COLUMN unit_amount REAL`,
		`ALTER TABLE entries ADD COLUMN custom_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE entries ADD COLUMN custom_calories REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE entries ADD COLUMN custom_protein REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE entries ADD COLUMN custom_carbs REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE entries ADD COLUMN custom_fat REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE entries ADD COLUMN custom_fiber REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE entries ADD COLUMN custom_sugar REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE entries ADD COLUMN custom_saturated_fat REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE entries ADD COLUMN custom_salt REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE entries ADD COLUMN custom_micros TEXT NOT NULL DEFAULT '{}'`,
		`ALTER TABLE daily_stats ADD COLUMN intake_incomplete INTEGER NOT NULL DEFAULT 0`,
	} {
		_, err = db.Exec(migration)
		if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
	}
	if err := ensureEntriesAllowCustom(db); err != nil {
		return err
	}
	_, err = db.Exec(`
		UPDATE entries
		SET unit_amount = COALESCE(
				(SELECT serving_size FROM foods WHERE foods.id = entries.food_id),
				amount
			),
			quantity = amount / COALESCE(
				NULLIF((SELECT serving_size FROM foods WHERE foods.id = entries.food_id), 0),
				amount
			)
		WHERE unit_amount IS NULL
	`)
	if err != nil {
		return err
	}
	return nil
}

func ensureEntriesAllowCustom(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(entries)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	foodIDNotNull := false
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == "food_id" {
			foodIDNotNull = notNull == 1
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if !foodIDNotNull {
		return nil
	}

	if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	defer db.Exec(`PRAGMA foreign_keys = ON`)

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`ALTER TABLE entries RENAME TO entries_old`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		CREATE TABLE entries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			food_id INTEGER REFERENCES foods(id) ON DELETE RESTRICT,
			entry_date TEXT NOT NULL,
			meal TEXT NOT NULL,
			amount REAL NOT NULL CHECK(amount > 0),
			quantity REAL NOT NULL DEFAULT 1,
			unit_amount REAL,
			custom_name TEXT NOT NULL DEFAULT '',
			custom_calories REAL NOT NULL DEFAULT 0,
			custom_protein REAL NOT NULL DEFAULT 0,
			custom_carbs REAL NOT NULL DEFAULT 0,
			custom_fat REAL NOT NULL DEFAULT 0,
			custom_fiber REAL NOT NULL DEFAULT 0,
			custom_sugar REAL NOT NULL DEFAULT 0,
			custom_saturated_fat REAL NOT NULL DEFAULT 0,
			custom_salt REAL NOT NULL DEFAULT 0,
			custom_micros TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO entries(
			id, food_id, entry_date, meal, amount, quantity, unit_amount,
			custom_name, custom_calories, custom_protein, custom_carbs, custom_fat,
			custom_fiber, custom_sugar, custom_saturated_fat, custom_salt,
			custom_micros, created_at
		)
		SELECT id, food_id, entry_date, meal, amount, quantity, unit_amount,
		       custom_name, custom_calories, custom_protein, custom_carbs, custom_fat,
		       custom_fiber, custom_sugar, custom_saturated_fat, custom_salt,
		       custom_micros, created_at
		FROM entries_old`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE entries_old`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS entries_date ON entries(entry_date)`); err != nil {
		return err
	}
	return tx.Commit()
}

func (a *app) health(w http.ResponseWriter, _ *http.Request) {
	if err := a.db.Ping(); err != nil {
		writeError(w, http.StatusServiceUnavailable, "Datenbank nicht erreichbar")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *app) listFoods(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	rows, err := a.db.Query(`
		SELECT id, name, brand, barcode, serving_size, serving_unit, calories,
		       protein, carbs, fat, fiber, sugar, saturated_fat, salt, micros,
		       source, image_url, serving_configured
		FROM foods
		WHERE ? = '' OR name LIKE '%' || ? || '%' OR brand LIKE '%' || ? || '%' OR barcode = ?
		ORDER BY CASE WHEN ? <> '' AND barcode = ? THEN 0 ELSE 1 END, name`,
		q, q, q, q, q, q)
	if err != nil {
		writeError(w, 500, "Lebensmittel konnten nicht geladen werden")
		return
	}
	defer rows.Close()

	items := make([]food, 0)
	for rows.Next() {
		f, err := scanFood(rows)
		if err != nil {
			writeError(w, 500, "Lebensmittel konnten nicht gelesen werden")
			return
		}
		items = append(items, f)
	}
	writeJSON(w, 200, items)
}

func (a *app) createFood(w http.ResponseWriter, r *http.Request) {
	var f food
	if err := decodeJSON(r, &f); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(f.Name) == "" {
		writeError(w, 400, "Name ist erforderlich")
		return
	}
	if f.ServingSize <= 0 {
		f.ServingSize = 100
	}
	if f.ServingUnit == "" {
		f.ServingUnit = "g"
	}
	if f.Source == "" {
		f.Source = "manual"
	}
	if f.Micros == nil {
		f.Micros = map[string]float64{}
	}
	id, err := a.insertFood(f)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			writeError(w, 409, "Dieser Barcode ist bereits gespeichert")
			return
		}
		writeError(w, 500, "Lebensmittel konnte nicht gespeichert werden")
		return
	}
	f.ID = id
	writeJSON(w, 201, f)
}

func (a *app) barcode(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.PathValue("barcode"))
	if len(code) < 8 || len(code) > 14 || !digitsOnly(code) {
		writeError(w, 400, "Ungültiger Barcode")
		return
	}

	if f, err := a.foodByBarcode(code); err == nil {
		writeJSON(w, 200, f)
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		writeError(w, 500, "Lokale Suche fehlgeschlagen")
		return
	}

	url := "https://world.openfoodfacts.org/api/v2/product/" + code +
		"?fields=code,product_name,brands,serving_quantity,serving_size,nutriments,image_front_small_url"
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
	req.Header.Set("User-Agent", a.userAgent)
	resp, err := a.client.Do(req)
	if err != nil {
		writeError(w, 502, "Open Food Facts ist gerade nicht erreichbar")
		return
	}
	defer resp.Body.Close()

	var payload struct {
		Status  int `json:"status"`
		Product struct {
			Name            string        `json:"product_name"`
			Brands          string        `json:"brands"`
			ServingQuantity float64       `json:"serving_quantity"`
			ServingSize     string        `json:"serving_size"`
			ImageURL        string        `json:"image_front_small_url"`
			Nutriments      offNutriments `json:"nutriments"`
		} `json:"product"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&payload) != nil {
		writeError(w, 502, "Ungültige Antwort von Open Food Facts")
		return
	}
	if payload.Status != 1 || payload.Product.Name == "" {
		writeError(w, 404, "Produkt nicht gefunden - du kannst es manuell anlegen")
		return
	}

	n := payload.Product.Nutriments
	f := food{
		Name:         payload.Product.Name,
		Brand:        payload.Product.Brands,
		Barcode:      code,
		ServingSize:  100,
		ServingUnit:  "g",
		Calories:     energyKcal(n),
		Protein:      n.Protein,
		Carbs:        n.Carbs,
		Fat:          n.Fat,
		Fiber:        n.Fiber,
		Sugar:        n.Sugar,
		SaturatedFat: n.SaturatedFat,
		Salt:         n.Salt,
		Micros:       extractMicros(n),
		Source:       "openfoodfacts",
		ImageURL:     payload.Product.ImageURL,
		NeedsServing: true,
	}
	if payload.Product.ServingQuantity > 0 {
		f.ServingSize = payload.Product.ServingQuantity
	}
	id, err := a.insertFood(f)
	if err != nil {
		if existing, findErr := a.foodByBarcode(code); findErr == nil {
			writeJSON(w, 200, existing)
			return
		}
		writeError(w, 500, "Produkt konnte nicht lokal gespeichert werden")
		return
	}
	f.ID = id
	writeJSON(w, 201, f)
}

func (a *app) insertFood(f food) (int64, error) {
	micros, _ := json.Marshal(f.Micros)
	result, err := a.db.Exec(`
		INSERT INTO foods(name, brand, barcode, serving_size, serving_unit, calories,
			protein, carbs, fat, fiber, sugar, saturated_fat, salt, micros, source,
			image_url, serving_configured)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(f.Name), strings.TrimSpace(f.Brand), strings.TrimSpace(f.Barcode),
		f.ServingSize, f.ServingUnit, nonNegative(f.Calories), nonNegative(f.Protein),
		nonNegative(f.Carbs), nonNegative(f.Fat), nonNegative(f.Fiber),
		nonNegative(f.Sugar), nonNegative(f.SaturatedFat), nonNegative(f.Salt),
		string(micros), f.Source, f.ImageURL, !f.NeedsServing)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (a *app) updateFood(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "Ungültige Lebensmittel-ID")
		return
	}
	var f food
	if err := decodeJSON(r, &f); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(f.Name) == "" {
		writeError(w, http.StatusBadRequest, "Name ist erforderlich")
		return
	}
	if f.ServingSize <= 0 || f.ServingSize > 10000 ||
		math.IsNaN(f.ServingSize) || math.IsInf(f.ServingSize, 0) {
		writeError(w, http.StatusBadRequest, "Die Standardmenge muss zwischen 0 und 10.000 g liegen")
		return
	}
	if f.Micros == nil {
		f.Micros = map[string]float64{}
	}
	for name, value := range f.Micros {
		if strings.TrimSpace(name) == "" || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			writeError(w, http.StatusBadRequest, "Ungültiger Mikronährstoffwert")
			return
		}
	}
	micros, _ := json.Marshal(f.Micros)
	result, err := a.db.Exec(`
		UPDATE foods
		SET name = ?, brand = ?, barcode = ?, serving_size = ?, serving_unit = 'g',
		    calories = ?, protein = ?, carbs = ?, fat = ?, fiber = ?, sugar = ?,
		    saturated_fat = ?, salt = ?, micros = ?, serving_configured = 1
		WHERE id = ?`,
		strings.TrimSpace(f.Name), strings.TrimSpace(f.Brand), strings.TrimSpace(f.Barcode),
		f.ServingSize, nonNegative(f.Calories), nonNegative(f.Protein),
		nonNegative(f.Carbs), nonNegative(f.Fat), nonNegative(f.Fiber),
		nonNegative(f.Sugar), nonNegative(f.SaturatedFat), nonNegative(f.Salt),
		string(micros), id,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			writeError(w, http.StatusConflict, "Dieser Barcode ist bereits einem anderen Lebensmittel zugeordnet")
			return
		}
		writeError(w, http.StatusInternalServerError, "Lebensmittel konnte nicht aktualisiert werden")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "Lebensmittel nicht gefunden")
		return
	}
	updated, err := a.foodByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Lebensmittel konnte nicht geladen werden")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (a *app) updateFoodServing(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "Ungültige Lebensmittel-ID")
		return
	}
	var input struct {
		ServingSize float64 `json:"servingSize"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.ServingSize <= 0 || input.ServingSize > 10000 ||
		math.IsNaN(input.ServingSize) || math.IsInf(input.ServingSize, 0) {
		writeError(w, http.StatusBadRequest, "Die Portionsgröße muss zwischen 0 und 10.000 g liegen")
		return
	}
	result, err := a.db.Exec(
		`UPDATE foods SET serving_size = ?, serving_unit = 'g', serving_configured = 1 WHERE id = ?`,
		input.ServingSize, id,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Portionsgröße konnte nicht gespeichert werden")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "Lebensmittel nicht gefunden")
		return
	}
	f, err := a.foodByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Lebensmittel konnte nicht geladen werden")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (a *app) foodByBarcode(code string) (food, error) {
	row := a.db.QueryRow(`
		SELECT id, name, brand, barcode, serving_size, serving_unit, calories,
		       protein, carbs, fat, fiber, sugar, saturated_fat, salt, micros,
		       source, image_url, serving_configured
		FROM foods WHERE barcode = ?`, code)
	return scanFood(row)
}

func (a *app) foodByID(id int64) (food, error) {
	row := a.db.QueryRow(`
		SELECT id, name, brand, barcode, serving_size, serving_unit, calories,
		       protein, carbs, fat, fiber, sugar, saturated_fat, salt, micros,
		       source, image_url, serving_configured
		FROM foods WHERE id = ?`, id)
	return scanFood(row)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanFood(s scanner) (food, error) {
	var f food
	var micros string
	var servingConfigured bool
	err := s.Scan(&f.ID, &f.Name, &f.Brand, &f.Barcode, &f.ServingSize,
		&f.ServingUnit, &f.Calories, &f.Protein, &f.Carbs, &f.Fat, &f.Fiber,
		&f.Sugar, &f.SaturatedFat, &f.Salt, &micros, &f.Source, &f.ImageURL,
		&servingConfigured)
	if err != nil {
		return f, err
	}
	f.Micros = map[string]float64{}
	_ = json.Unmarshal([]byte(micros), &f.Micros)
	f.NeedsServing = !servingConfigured
	return f, nil
}

func (a *app) listEntries(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if !validDate(date) {
		writeError(w, 400, "Datum muss YYYY-MM-DD entsprechen")
		return
	}
	rows, err := a.db.Query(`
		SELECT e.id, e.food_id, e.entry_date, e.meal, e.amount,
		       e.quantity, COALESCE(e.unit_amount, e.amount), e.created_at,
		       e.food_id IS NULL,
		       COALESCE(f.id, 0),
		       CASE WHEN e.food_id IS NULL THEN e.custom_name ELSE f.name END,
		       COALESCE(f.brand, ''),
		       COALESCE(f.barcode, ''),
		       CASE WHEN e.food_id IS NULL THEN 100 ELSE f.serving_size END,
		       COALESCE(f.serving_unit, 'g'),
		       CASE WHEN e.food_id IS NULL THEN e.custom_calories ELSE f.calories END,
		       CASE WHEN e.food_id IS NULL THEN e.custom_protein ELSE f.protein END,
		       CASE WHEN e.food_id IS NULL THEN e.custom_carbs ELSE f.carbs END,
		       CASE WHEN e.food_id IS NULL THEN e.custom_fat ELSE f.fat END,
		       CASE WHEN e.food_id IS NULL THEN e.custom_fiber ELSE f.fiber END,
		       CASE WHEN e.food_id IS NULL THEN e.custom_sugar ELSE f.sugar END,
		       CASE WHEN e.food_id IS NULL THEN e.custom_saturated_fat ELSE f.saturated_fat END,
		       CASE WHEN e.food_id IS NULL THEN e.custom_salt ELSE f.salt END,
		       CASE WHEN e.food_id IS NULL THEN e.custom_micros ELSE f.micros END,
		       CASE WHEN e.food_id IS NULL THEN 'quick' ELSE f.source END,
		       COALESCE(f.image_url, ''),
		       COALESCE(f.serving_configured, 1)
		FROM entries e LEFT JOIN foods f ON f.id = e.food_id
		WHERE e.entry_date = ?
		ORDER BY CASE e.meal WHEN 'breakfast' THEN 1 WHEN 'lunch' THEN 2
		             WHEN 'dinner' THEN 3 WHEN 'snack' THEN 4
		             WHEN 'drinks' THEN 5 ELSE 6 END, e.created_at`, date)
	if err != nil {
		writeError(w, 500, "Einträge konnten nicht geladen werden")
		return
	}
	defer rows.Close()
	items := make([]entry, 0)
	for rows.Next() {
		var e entry
		var foodID sql.NullInt64
		var micros string
		var servingConfigured bool
		err := rows.Scan(&e.ID, &foodID, &e.Date, &e.Meal, &e.Amount,
			&e.Quantity, &e.UnitAmount, &e.CreatedAt, &e.IsCustom,
			&e.Food.ID, &e.Food.Name, &e.Food.Brand, &e.Food.Barcode,
			&e.Food.ServingSize, &e.Food.ServingUnit, &e.Food.Calories,
			&e.Food.Protein, &e.Food.Carbs, &e.Food.Fat, &e.Food.Fiber,
			&e.Food.Sugar, &e.Food.SaturatedFat, &e.Food.Salt, &micros,
			&e.Food.Source, &e.Food.ImageURL, &servingConfigured)
		if err != nil {
			writeError(w, 500, "Einträge konnten nicht gelesen werden")
			return
		}
		if foodID.Valid {
			id := foodID.Int64
			e.FoodID = &id
		}
		e.Food.Micros = map[string]float64{}
		_ = json.Unmarshal([]byte(micros), &e.Food.Micros)
		e.Food.NeedsServing = !servingConfigured
		items = append(items, e)
	}
	writeJSON(w, 200, items)
}

func (a *app) createEntry(w http.ResponseWriter, r *http.Request) {
	var input struct {
		FoodID       *int64             `json:"foodId"`
		Date         string             `json:"date"`
		Meal         string             `json:"meal"`
		Amount       float64            `json:"amount"`
		Quantity     float64            `json:"quantity"`
		UnitAmount   float64            `json:"unitAmount"`
		Name         string             `json:"name"`
		Calories     float64            `json:"calories"`
		Protein      float64            `json:"protein"`
		Carbs        float64            `json:"carbs"`
		Fat          float64            `json:"fat"`
		Fiber        float64            `json:"fiber"`
		Sugar        float64            `json:"sugar"`
		SaturatedFat float64            `json:"saturatedFat"`
		Salt         float64            `json:"salt"`
		Micros       map[string]float64 `json:"micros"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if !validDate(input.Date) {
		writeError(w, 400, "Datum muss YYYY-MM-DD entsprechen")
		return
	}
	if !validMeal(input.Meal) {
		writeError(w, 400, "Ungültige Mahlzeit")
		return
	}
	if input.FoodID == nil {
		f := food{
			Name:         input.Name,
			ServingSize:  100,
			ServingUnit:  "g",
			Calories:     input.Calories,
			Protein:      input.Protein,
			Carbs:        input.Carbs,
			Fat:          input.Fat,
			Fiber:        input.Fiber,
			Sugar:        input.Sugar,
			SaturatedFat: input.SaturatedFat,
			Salt:         input.Salt,
			Micros:       input.Micros,
			Source:       "quick",
		}
		if err := validateCustomEntryFood(f); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		if f.Micros == nil {
			f.Micros = map[string]float64{}
		}
		micros, _ := json.Marshal(f.Micros)
		result, err := a.db.Exec(`
			INSERT INTO entries(
				food_id, entry_date, meal, amount, quantity, unit_amount,
				custom_name, custom_calories, custom_protein, custom_carbs, custom_fat,
				custom_fiber, custom_sugar, custom_saturated_fat, custom_salt, custom_micros
			)
			VALUES (NULL, ?, ?, 100, 1, 100, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			input.Date, input.Meal, strings.TrimSpace(f.Name),
			nonNegative(f.Calories), nonNegative(f.Protein), nonNegative(f.Carbs),
			nonNegative(f.Fat), nonNegative(f.Fiber), nonNegative(f.Sugar),
			nonNegative(f.SaturatedFat), nonNegative(f.Salt), string(micros),
		)
		if err != nil {
			writeError(w, 500, "Freier Eintrag konnte nicht gespeichert werden")
			return
		}
		id, _ := result.LastInsertId()
		writeJSON(w, 201, map[string]int64{"id": id})
		return
	}
	if *input.FoodID < 1 || input.Amount <= 0 || input.Amount > 10000 {
		writeError(w, 400, "Lebensmittel und eine gültige Menge sind erforderlich")
		return
	}
	input.Quantity, input.UnitAmount = normalizedQuantity(input.Amount, input.Quantity, input.UnitAmount)
	result, err := a.db.Exec(
		`INSERT INTO entries(food_id, entry_date, meal, amount, quantity, unit_amount)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		*input.FoodID, input.Date, input.Meal, input.Amount, input.Quantity, input.UnitAmount)
	if err != nil {
		writeError(w, 400, "Eintrag konnte nicht gespeichert werden")
		return
	}
	id, _ := result.LastInsertId()
	writeJSON(w, 201, map[string]int64{"id": id})
}

func (a *app) updateEntry(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "Ungültige ID")
		return
	}
	var input struct {
		Meal         string             `json:"meal"`
		Amount       float64            `json:"amount"`
		Quantity     float64            `json:"quantity"`
		UnitAmount   float64            `json:"unitAmount"`
		Name         string             `json:"name"`
		Calories     float64            `json:"calories"`
		Protein      float64            `json:"protein"`
		Carbs        float64            `json:"carbs"`
		Fat          float64            `json:"fat"`
		Fiber        float64            `json:"fiber"`
		Sugar        float64            `json:"sugar"`
		SaturatedFat float64            `json:"saturatedFat"`
		Salt         float64            `json:"salt"`
		Micros       map[string]float64 `json:"micros"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !validMeal(input.Meal) {
		writeError(w, http.StatusBadRequest, "Ungültige Mahlzeit")
		return
	}

	var foodID sql.NullInt64
	if err := a.db.QueryRow(`SELECT food_id FROM entries WHERE id = ?`, id).Scan(&foodID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "Eintrag nicht gefunden")
			return
		}
		writeError(w, http.StatusInternalServerError, "Eintrag konnte nicht geladen werden")
		return
	}
	if !foodID.Valid {
		f := food{
			Name:         input.Name,
			Calories:     input.Calories,
			Protein:      input.Protein,
			Carbs:        input.Carbs,
			Fat:          input.Fat,
			Fiber:        input.Fiber,
			Sugar:        input.Sugar,
			SaturatedFat: input.SaturatedFat,
			Salt:         input.Salt,
			Micros:       input.Micros,
		}
		if err := validateCustomEntryFood(f); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if f.Micros == nil {
			f.Micros = map[string]float64{}
		}
		micros, _ := json.Marshal(f.Micros)
		result, err := a.db.Exec(`
			UPDATE entries
			SET meal = ?, custom_name = ?, custom_calories = ?, custom_protein = ?,
			    custom_carbs = ?, custom_fat = ?, custom_fiber = ?, custom_sugar = ?,
			    custom_saturated_fat = ?, custom_salt = ?, custom_micros = ?
			WHERE id = ?`,
			input.Meal, strings.TrimSpace(f.Name), nonNegative(f.Calories),
			nonNegative(f.Protein), nonNegative(f.Carbs), nonNegative(f.Fat),
			nonNegative(f.Fiber), nonNegative(f.Sugar), nonNegative(f.SaturatedFat),
			nonNegative(f.Salt), string(micros), id,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Eintrag konnte nicht aktualisiert werden")
			return
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			writeError(w, http.StatusNotFound, "Eintrag nicht gefunden")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if input.Amount <= 0 || input.Amount > 10000 ||
		math.IsNaN(input.Amount) || math.IsInf(input.Amount, 0) {
		writeError(w, http.StatusBadRequest, "Die Menge muss zwischen 0 und 10.000 g liegen")
		return
	}
	input.Quantity, input.UnitAmount = normalizedQuantity(input.Amount, input.Quantity, input.UnitAmount)
	result, err := a.db.Exec(
		`UPDATE entries SET meal = ?, amount = ?, quantity = ?, unit_amount = ? WHERE id = ?`,
		input.Meal, input.Amount, input.Quantity, input.UnitAmount, id,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Eintrag konnte nicht aktualisiert werden")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "Eintrag nicht gefunden")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) deleteEntry(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, 400, "Ungültige ID")
		return
	}
	result, err := a.db.Exec(`DELETE FROM entries WHERE id = ?`, id)
	if err != nil {
		writeError(w, 500, "Eintrag konnte nicht gelöscht werden")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		writeError(w, 404, "Eintrag nicht gefunden")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) getGoals(w http.ResponseWriter, _ *http.Request) {
	var raw string
	if err := a.db.QueryRow(`SELECT value FROM settings WHERE key = 'goals'`).Scan(&raw); err != nil {
		writeError(w, 500, "Ziele konnten nicht geladen werden")
		return
	}
	var g goals
	_ = json.Unmarshal([]byte(raw), &g)
	writeJSON(w, 200, g)
}

func (a *app) putGoals(w http.ResponseWriter, r *http.Request) {
	var g goals
	if err := decodeJSON(r, &g); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if g.Calories <= 0 || g.Protein < 0 || g.Carbs < 0 || g.Fat < 0 || g.Fiber < 0 {
		writeError(w, 400, "Ziele müssen positive Werte sein")
		return
	}
	raw, _ := json.Marshal(g)
	_, err := a.db.Exec(`UPDATE settings SET value = ? WHERE key = 'goals'`, string(raw))
	if err != nil {
		writeError(w, 500, "Ziele konnten nicht gespeichert werden")
		return
	}
	writeJSON(w, 200, g)
}

func (a *app) getDailyStats(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if !validDate(date) {
		writeError(w, http.StatusBadRequest, "Datum muss YYYY-MM-DD entsprechen")
		return
	}
	stats := dailyStats{Date: date}
	err := a.db.QueryRow(`
		SELECT weight, calories_burned, intake_incomplete
		FROM daily_stats WHERE entry_date = ?`, date,
	).Scan(&stats.Weight, &stats.CaloriesBurned, &stats.IntakeIncomplete)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "Tageswerte konnten nicht geladen werden")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (a *app) putDailyStats(w http.ResponseWriter, r *http.Request) {
	var stats dailyStats
	if err := decodeJSON(r, &stats); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !validDate(stats.Date) {
		writeError(w, http.StatusBadRequest, "Datum muss YYYY-MM-DD entsprechen")
		return
	}
	if stats.Weight != nil &&
		(*stats.Weight <= 0 || *stats.Weight > 500 || math.IsNaN(*stats.Weight) || math.IsInf(*stats.Weight, 0)) {
		writeError(w, http.StatusBadRequest, "Das Gewicht muss zwischen 0 und 500 kg liegen")
		return
	}
	if stats.Weight != nil {
		rounded := math.Round(*stats.Weight*10) / 10
		stats.Weight = &rounded
	}
	if stats.CaloriesBurned != nil &&
		(*stats.CaloriesBurned < 0 || *stats.CaloriesBurned > 20000 ||
			math.IsNaN(*stats.CaloriesBurned) || math.IsInf(*stats.CaloriesBurned, 0)) {
		writeError(w, http.StatusBadRequest, "Der Verbrauch muss zwischen 0 und 20.000 kcal liegen")
		return
	}
	if stats.Weight == nil && stats.CaloriesBurned == nil && !stats.IntakeIncomplete {
		if _, err := a.db.Exec(`DELETE FROM daily_stats WHERE entry_date = ?`, stats.Date); err != nil {
			writeError(w, http.StatusInternalServerError, "Tageswerte konnten nicht gelöscht werden")
			return
		}
		writeJSON(w, http.StatusOK, stats)
		return
	}
	_, err := a.db.Exec(`
		INSERT INTO daily_stats(entry_date, weight, calories_burned, intake_incomplete, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(entry_date) DO UPDATE SET
			weight = excluded.weight,
			calories_burned = excluded.calories_burned,
			intake_incomplete = excluded.intake_incomplete,
			updated_at = CURRENT_TIMESTAMP`,
		stats.Date, stats.Weight, stats.CaloriesBurned, stats.IntakeIncomplete,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Tageswerte konnten nicht gespeichert werden")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (a *app) getHistory(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	fromDate, fromErr := time.Parse("2006-01-02", from)
	toDate, toErr := time.Parse("2006-01-02", to)
	if fromErr != nil || toErr != nil || fromDate.After(toDate) {
		writeError(w, http.StatusBadRequest, "Gültiges Start- und Enddatum sind erforderlich")
		return
	}
	if toDate.Sub(fromDate) > 366*24*time.Hour {
		writeError(w, http.StatusBadRequest, "Der Zeitraum darf höchstens 366 Tage umfassen")
		return
	}

	rows, err := a.db.Query(`
		WITH entry_totals AS (
			SELECT e.entry_date,
			       SUM(CASE
			           WHEN e.food_id IS NULL THEN e.custom_calories
			           ELSE e.amount / 100.0 * f.calories
			       END) AS calories_in
			FROM entries e
			LEFT JOIN foods f ON f.id = e.food_id
			WHERE e.entry_date BETWEEN ? AND ?
			GROUP BY e.entry_date
		)
		SELECT d.entry_date AS entry_date,
		       et.calories_in,
		       d.weight,
		       d.calories_burned,
		       d.intake_incomplete
		FROM daily_stats d
		LEFT JOIN entry_totals et ON et.entry_date = d.entry_date
		WHERE d.entry_date BETWEEN ? AND ?
		UNION
		SELECT et.entry_date AS entry_date,
		       et.calories_in,
		       NULL AS weight,
		       NULL AS calories_burned,
		       0 AS intake_incomplete
		FROM entry_totals et
		LEFT JOIN daily_stats d ON d.entry_date = et.entry_date
		WHERE d.entry_date IS NULL
		ORDER BY entry_date`,
		from, to, from, to,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Verlauf konnte nicht geladen werden")
		return
	}
	defer rows.Close()

	byDate := map[string]historyPoint{}
	for rows.Next() {
		var point historyPoint
		if err := rows.Scan(
			&point.Date, &point.CaloriesIn, &point.Weight,
			&point.CaloriesBurned, &point.IntakeIncomplete,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "Verlauf konnte nicht gelesen werden")
			return
		}
		byDate[point.Date] = point
	}

	points := make([]historyPoint, 0, int(toDate.Sub(fromDate).Hours()/24)+1)
	for date := fromDate; !date.After(toDate); date = date.AddDate(0, 0, 1) {
		key := date.Format("2006-01-02")
		point, ok := byDate[key]
		if !ok {
			point = historyPoint{Date: key}
		}
		points = append(points, point)
	}
	writeJSON(w, http.StatusOK, points)
}

func spaHandler() http.Handler {
	sub, err := fs.Sub(webFiles, "web")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sw.js" {
			w.Header().Set("Cache-Control", "no-cache")
		}
		if r.URL.Path != "/" {
			if _, err := fs.Stat(sub, strings.TrimPrefix(path.Clean(r.URL.Path), "/")); err == nil {
				files.ServeHTTP(w, r)
				return
			}
		}
		r.URL.Path = "/"
		files.ServeHTTP(w, r)
	})
}

func extractMicros(n offNutriments) map[string]float64 {
	// Values are normalized to milligrams per 100 g for a consistent UI.
	values := map[string]float64{
		"Natrium":     n.Sodium,
		"Calcium":     n.Calcium,
		"Eisen":       n.Iron,
		"Magnesium":   n.Magnesium,
		"Kalium":      n.Potassium,
		"Zink":        n.Zinc,
		"Vitamin C":   n.VitaminC,
		"Vitamin B12": n.VitaminB12,
		"Vitamin D":   n.VitaminD,
	}
	result := map[string]float64{}
	for name, value := range values {
		if value > 0 {
			result[name] = value * 1000
		}
	}
	return result
}

func energyKcal(n offNutriments) float64 {
	if n.EnergyKcal > 0 {
		return n.EnergyKcal
	}
	return n.Energy / 4.184
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("ungültige Eingabe: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func basicAuth(next http.Handler) http.Handler {
	username := os.Getenv("AUTH_USERNAME")
	password := os.Getenv("AUTH_PASSWORD")
	if username == "" || password == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}
		givenUser, givenPassword, ok := r.BasicAuth()
		userMatches := subtle.ConstantTimeCompare([]byte(givenUser), []byte(username)) == 1
		passwordMatches := subtle.ConstantTimeCompare([]byte(givenPassword), []byte(password)) == 1
		if !ok || !userMatches || !passwordMatches {
			w.Header().Set("WWW-Authenticate", `Basic realm="MampfTracker", charset="UTF-8"`)
			writeError(w, http.StatusUnauthorized, "Anmeldung erforderlich")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func validDate(value string) bool {
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}

func digitsOnly(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func nonNegative(value float64) float64 {
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

func validateCustomEntryFood(f food) error {
	if strings.TrimSpace(f.Name) == "" {
		return errors.New("Name ist erforderlich")
	}
	if !validNutrientValue(f.Calories, 20000) {
		return errors.New("Kalorien müssen zwischen 0 und 20.000 liegen")
	}
	for _, item := range []struct {
		label string
		value float64
	}{
		{"Protein", f.Protein},
		{"Kohlenhydrate", f.Carbs},
		{"Fett", f.Fat},
		{"Ballaststoffe", f.Fiber},
		{"Zucker", f.Sugar},
		{"Gesättigte Fettsäuren", f.SaturatedFat},
		{"Salz", f.Salt},
	} {
		if !validNutrientValue(item.value, 10000) {
			return fmt.Errorf("%s muss zwischen 0 und 10.000 liegen", item.label)
		}
	}
	if f.Micros == nil {
		return nil
	}
	for name, value := range f.Micros {
		if strings.TrimSpace(name) == "" || !validNutrientValue(value, 100000) {
			return errors.New("Ungültiger Mikronährstoffwert")
		}
	}
	return nil
}

func validNutrientValue(value, max float64) bool {
	return value >= 0 && value <= max && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func validMeal(value string) bool {
	return contains([]string{"breakfast", "lunch", "dinner", "snack", "drinks"}, value)
}

func normalizedQuantity(amount, quantity, unitAmount float64) (float64, float64) {
	if quantity <= 0 || unitAmount <= 0 ||
		math.IsNaN(quantity) || math.IsInf(quantity, 0) ||
		math.IsNaN(unitAmount) || math.IsInf(unitAmount, 0) ||
		math.Abs(quantity*unitAmount-amount) > 0.01 {
		return 1, amount
	}
	return quantity, unitAmount
}
