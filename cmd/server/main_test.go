package main

import (
	"bytes"
	"database/sql"
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
