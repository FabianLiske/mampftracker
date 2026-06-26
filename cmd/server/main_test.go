package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestMigrateSeedsGoals(t *testing.T) {
	db := testDB(t)
	var value string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'goals'`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value == "" {
		t.Fatal("goals setting was empty")
	}
}

func TestFoodRoundTrip(t *testing.T) {
	a := &app{db: testDB(t)}
	want := food{
		Name:        "Testmampf",
		Barcode:     "12345678",
		ServingSize: 100,
		ServingUnit: "g",
		Calories:    123,
		Protein:     4.5,
		Micros:      map[string]float64{"Eisen": 2.1},
		Source:      "manual",
	}
	id, err := a.insertFood(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.foodByBarcode(want.Barcode)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != id || got.Name != want.Name || got.Micros["Eisen"] != 2.1 {
		t.Fatalf("unexpected food: %#v", got)
	}
}

func TestHelpers(t *testing.T) {
	if !validDate("2026-06-20") || validDate("20.06.2026") {
		t.Fatal("date validation returned an unexpected result")
	}
	if !digitsOnly("12345678") || digitsOnly("1234x678") {
		t.Fatal("barcode validation returned an unexpected result")
	}
	if !validMeal("drinks") || validMeal("brunch") {
		t.Fatal("meal validation returned an unexpected result")
	}
}

func TestBasicAuth(t *testing.T) {
	t.Setenv("AUTH_USERNAME", "mampf")
	t.Setenv("AUTH_PASSWORD", "secret")
	handler := basicAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetBasicAuth("mampf", "secret")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("expected success, got %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/health", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("health endpoint should bypass auth, got %d", response.Code)
	}
}

func TestUpdateFoodServing(t *testing.T) {
	a := &app{db: testDB(t)}
	id, err := a.insertFood(food{
		Name: "Brezel", ServingSize: 100, ServingUnit: "g",
		Micros: map[string]float64{}, Source: "openfoodfacts", NeedsServing: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := a.foodByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if !before.NeedsServing {
		t.Fatal("new imported food should require a serving setup")
	}

	request := httptest.NewRequest(
		http.MethodPut,
		"/api/foods/1/serving",
		bytes.NewBufferString(`{"servingSize":80}`),
	)
	request.SetPathValue("id", strconv.FormatInt(id, 10))
	response := httptest.NewRecorder()
	a.updateFoodServing(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected success, got %d: %s", response.Code, response.Body.String())
	}

	got, err := a.foodByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ServingSize != 80 {
		t.Fatalf("expected an 80 g serving, got %v", got.ServingSize)
	}
	if got.NeedsServing {
		t.Fatal("serving setup should be complete after updating the default amount")
	}
}

func TestUpdateEntry(t *testing.T) {
	a := &app{db: testDB(t)}
	foodID, err := a.insertFood(food{
		Name: "Obazda", ServingSize: 125, ServingUnit: "g",
		Micros: map[string]float64{}, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := a.db.Exec(
		`INSERT INTO entries(food_id, entry_date, meal, amount) VALUES (?, ?, ?, ?)`,
		foodID, "2026-06-20", "breakfast", 125,
	)
	if err != nil {
		t.Fatal(err)
	}
	entryID, _ := result.LastInsertId()

	request := httptest.NewRequest(
		http.MethodPut,
		"/api/entries/1",
		bytes.NewBufferString(`{"meal":"lunch","amount":250,"quantity":2,"unitAmount":125}`),
	)
	request.SetPathValue("id", strconv.FormatInt(entryID, 10))
	response := httptest.NewRecorder()
	a.updateEntry(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("expected no content, got %d: %s", response.Code, response.Body.String())
	}

	var meal string
	var amount, quantity, unitAmount float64
	if err := a.db.QueryRow(
		`SELECT meal, amount, quantity, unit_amount FROM entries WHERE id = ?`,
		entryID,
	).Scan(&meal, &amount, &quantity, &unitAmount); err != nil {
		t.Fatal(err)
	}
	if meal != "lunch" || amount != 250 || quantity != 2 || unitAmount != 125 {
		t.Fatalf("unexpected updated entry: meal=%s amount=%v quantity=%v unit=%v",
			meal, amount, quantity, unitAmount)
	}
}

func TestCustomEntryRoundTrip(t *testing.T) {
	a := &app{db: testDB(t)}
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/entries",
		bytes.NewBufferString(`{
			"date":"2026-06-20","meal":"dinner","name":"Buffet",
			"calories":1200,"protein":45.5,"carbs":150,"fat":42,
			"fiber":8,"sugar":20,"saturatedFat":12,"salt":4.2,
			"micros":{"Eisen":3.5}
		}`),
	)
	response := httptest.NewRecorder()
	a.createEntry(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("expected created, got %d: %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/entries?date=2026-06-20", nil)
	response = httptest.NewRecorder()
	a.listEntries(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected success, got %d: %s", response.Code, response.Body.String())
	}
	var entries []entry
	if err := json.NewDecoder(response.Body).Decode(&entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(entries))
	}
	got := entries[0]
	if got.FoodID != nil || !got.IsCustom || got.Food.Name != "Buffet" ||
		got.Food.Calories != 1200 || got.Food.Micros["Eisen"] != 3.5 {
		t.Fatalf("unexpected custom entry: %#v", got)
	}

	request = httptest.NewRequest(
		http.MethodPut,
		"/api/entries/1",
		bytes.NewBufferString(`{
			"meal":"lunch","name":"Brunch-Buffet",
			"calories":900,"protein":35,"carbs":100,"fat":30,
			"fiber":7,"sugar":16,"saturatedFat":8,"salt":3,
			"micros":{"Eisen":2}
		}`),
	)
	request.SetPathValue("id", strconv.FormatInt(got.ID, 10))
	response = httptest.NewRecorder()
	a.updateEntry(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("expected no content, got %d: %s", response.Code, response.Body.String())
	}
	var name, meal string
	var calories float64
	if err := a.db.QueryRow(
		`SELECT meal, custom_name, custom_calories FROM entries WHERE id = ?`,
		got.ID,
	).Scan(&meal, &name, &calories); err != nil {
		t.Fatal(err)
	}
	if meal != "lunch" || name != "Brunch-Buffet" || calories != 900 {
		t.Fatalf("unexpected updated custom entry: meal=%s name=%s calories=%v", meal, name, calories)
	}
}

func TestUpdateFoodAffectsExistingEntries(t *testing.T) {
	a := &app{db: testDB(t)}
	foodID, err := a.insertFood(food{
		Name: "Alte Brezel", ServingSize: 80, ServingUnit: "g",
		Calories: 250, Micros: map[string]float64{}, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.Exec(
		`INSERT INTO entries(food_id, entry_date, meal, amount) VALUES (?, ?, ?, ?)`,
		foodID, "2026-06-20", "breakfast", 160,
	); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(
		http.MethodPut,
		"/api/foods/1",
		bytes.NewBufferString(`{
			"id":1,"name":"Neue Brezel","brand":"Bäcker","barcode":"",
			"servingSize":80,"servingUnit":"g","calories":300,"protein":9,
			"carbs":55,"fat":4,"fiber":3,"sugar":2,"saturatedFat":1,
			"salt":2,"micros":{"Eisen":1.5},"source":"manual","imageUrl":""
		}`),
	)
	request.SetPathValue("id", strconv.FormatInt(foodID, 10))
	response := httptest.NewRecorder()
	a.updateFood(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected success, got %d: %s", response.Code, response.Body.String())
	}

	var name string
	var calories, amount float64
	if err := a.db.QueryRow(`
		SELECT f.name, f.calories, e.amount
		FROM entries e JOIN foods f ON f.id = e.food_id
		WHERE e.food_id = ?`, foodID).Scan(&name, &calories, &amount); err != nil {
		t.Fatal(err)
	}
	if name != "Neue Brezel" || calories != 300 || amount != 160 {
		t.Fatalf("unexpected historical view: name=%s calories=%v amount=%v", name, calories, amount)
	}
}

func TestDailyStatsRoundTrip(t *testing.T) {
	a := &app{db: testDB(t)}
	request := httptest.NewRequest(
		http.MethodPut,
		"/api/daily-stats",
		bytes.NewBufferString(`{"date":"2026-06-20","weight":82.44,"caloriesBurned":2750,"intakeIncomplete":true}`),
	)
	response := httptest.NewRecorder()
	a.putDailyStats(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected success, got %d: %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/daily-stats?date=2026-06-20", nil)
	response = httptest.NewRecorder()
	a.getDailyStats(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected success, got %d: %s", response.Code, response.Body.String())
	}
	var stats dailyStats
	if err := json.NewDecoder(response.Body).Decode(&stats); err != nil {
		t.Fatal(err)
	}
	if stats.Weight == nil || *stats.Weight != 82.4 ||
		stats.CaloriesBurned == nil || *stats.CaloriesBurned != 2750 ||
		!stats.IntakeIncomplete {
		t.Fatalf("unexpected daily stats: %#v", stats)
	}

	request = httptest.NewRequest(
		http.MethodPut,
		"/api/daily-stats",
		bytes.NewBufferString(`{"date":"2026-06-20","weight":null,"caloriesBurned":null,"intakeIncomplete":false}`),
	)
	response = httptest.NewRecorder()
	a.putDailyStats(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected delete success, got %d: %s", response.Code, response.Body.String())
	}
	var count int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM daily_stats`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected the empty daily record to be deleted, got %d rows", count)
	}
}

func TestHistoryIncludesRangeAndGaps(t *testing.T) {
	a := &app{db: testDB(t)}
	foodID, err := a.insertFood(food{
		Name: "Testessen", ServingSize: 100, ServingUnit: "g",
		Calories: 500, Micros: map[string]float64{}, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.Exec(
		`INSERT INTO entries(food_id, entry_date, meal, amount) VALUES (?, ?, ?, ?)`,
		foodID, "2026-06-19", "dinner", 200,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.Exec(
		`INSERT INTO entries(food_id, entry_date, meal, amount, custom_name, custom_calories)
		 VALUES (NULL, ?, ?, 100, ?, ?)`,
		"2026-06-19", "snack", "Freier Test", 300,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.Exec(
		`INSERT INTO daily_stats(entry_date, weight, calories_burned) VALUES (?, ?, ?)`,
		"2026-06-20", 81.5, 2600,
	); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/history?from=2026-06-18&to=2026-06-20",
		nil,
	)
	response := httptest.NewRecorder()
	a.getHistory(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected success, got %d: %s", response.Code, response.Body.String())
	}
	var points []historyPoint
	if err := json.NewDecoder(response.Body).Decode(&points); err != nil {
		t.Fatal(err)
	}
	if len(points) != 3 {
		t.Fatalf("expected 3 days, got %d", len(points))
	}
	if points[0].CaloriesIn != nil || points[0].Weight != nil {
		t.Fatalf("expected an empty first day: %#v", points[0])
	}
	if points[1].CaloriesIn == nil || *points[1].CaloriesIn != 1300 {
		t.Fatalf("expected 1300 kcal intake: %#v", points[1])
	}
	if points[2].Weight == nil || *points[2].Weight != 81.5 ||
		points[2].CaloriesBurned == nil || *points[2].CaloriesBurned != 2600 {
		t.Fatalf("unexpected daily values: %#v", points[2])
	}
}
