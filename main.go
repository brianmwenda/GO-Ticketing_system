package main

import (
	"html/template"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Reuse booking structs from CLI version
type Conference struct {
	Name             string
	TotalTickets     int
	RemainingTickets int
}

type Booking struct {
	ID        int
	FirstName string
	LastName  string
	Email     string
	Tickets   int
	BookedAt  time.Time
}

var (
	conf     = Conference{Name: "Go Conference", TotalTickets: 100, RemainingTickets: 100}
	bookings []Booking
	nextID   = 1
	mu       sync.Mutex
)

// Templates
var tpl = template.Must(template.New("main").Parse(`
<!DOCTYPE html>
<html>
<head>
	<title>{{.Conf.Name}} - Ticketing</title>
	<script src="https://cdn.tailwindcss.com"></script>
</head>
<body class="bg-gray-100 p-6">
	<div class="max-w-3xl mx-auto bg-white shadow-lg rounded-xl p-6">
		<h1 class="text-2xl font-bold mb-4">🎟️ {{.Conf.Name}}</h1>
		<p>Total: {{.Conf.TotalTickets}} | Remaining: {{.Conf.RemainingTickets}}</p>

		<h2 class="text-xl font-semibold mt-6">Book Tickets</h2>
		<form method="POST" action="/book" class="space-y-2">
			<input name="first" placeholder="First Name" class="border p-2 w-full rounded" required>
			<input name="last" placeholder="Last Name" class="border p-2 w-full rounded" required>
			<input name="email" placeholder="Email" type="email" class="border p-2 w-full rounded" required>
			<input name="tickets" placeholder="Tickets" type="number" min="1" class="border p-2 w-full rounded" required>
			<button class="bg-blue-600 text-white px-4 py-2 rounded">Book</button>
		</form>

		<h2 class="text-xl font-semibold mt-6">Bookings</h2>
		<table class="w-full border mt-2">
			<tr class="bg-gray-200">
				<th class="p-2 border">ID</th>
				<th class="p-2 border">Name</th>
				<th class="p-2 border">Email</th>
				<th class="p-2 border">Tickets</th>
				<th class="p-2 border">Actions</th>
			</tr>
			{{range .Bookings}}
			<tr>
				<td class="p-2 border">{{.ID}}</td>
				<td class="p-2 border">{{.FirstName}} {{.LastName}}</td>
				<td class="p-2 border">{{.Email}}</td>
				<td class="p-2 border">{{.Tickets}}</td>
				<td class="p-2 border">
					<form method="POST" action="/cancel" style="display:inline;">
						<input type="hidden" name="id" value="{{.ID}}">
						<button class="bg-red-600 text-white px-2 py-1 rounded">Cancel</button>
					</form>
				</td>
			</tr>
			{{end}}
		</table>
	</div>
</body>
</html>
`))

// Handlers
func index(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()
	tpl.Execute(w, map[string]any{
		"Conf":     conf,
		"Bookings": bookings,
	})
}

func book(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	mu.Lock()
	defer mu.Unlock()

	tk, _ := strconv.Atoi(r.FormValue("tickets"))
	if tk <= 0 || tk > conf.RemainingTickets {
		http.Error(w, "Invalid ticket count", http.StatusBadRequest)
		return
	}

	b := Booking{
		ID:        nextID,
		FirstName: r.FormValue("first"),
		LastName:  r.FormValue("last"),
		Email:     r.FormValue("email"),
		Tickets:   tk,
		BookedAt:  time.Now(),
	}
	nextID++
	conf.RemainingTickets -= tk
	bookings = append(bookings, b)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func cancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	id, _ := strconv.Atoi(r.FormValue("id"))

	mu.Lock()
	defer mu.Unlock()
	for i, b := range bookings {
		if b.ID == id {
			conf.RemainingTickets += b.Tickets
			bookings = append(bookings[:i], bookings[i+1:]...)
			break
		}
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func main() {
	http.HandleFunc("/", index)
	http.HandleFunc("/book", book)
	http.HandleFunc("/cancel", cancel)

	println("🚀 Running at http://localhost:8080")
	http.ListenAndServe(":8080", nil)
}
