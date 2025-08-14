package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// ----------------------------
// Data Models
// ----------------------------

type Conference struct {
	Name             string `json:"name"`
	TotalTickets     int    `json:"total_tickets"`
	RemainingTickets int    `json:"remaining_tickets"`
}

type Booking struct {
	ID        uint64 `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Tickets   int    `json:"tickets"`
	BookedAt  int64  `json:"booked_at_unix"`
}

type AppState struct {
	Conference Conference `json:"conference"`
	Bookings   []Booking  `json:"bookings"`
	NextID     uint64     `json:"next_id"`
}

// ----------------------------
// Globals (kept minimal & encapsulated)
// ----------------------------

var (
	stateFile  = "bookings.json"
	emailRegex = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
)

// ----------------------------
// Persistence
// ----------------------------

func loadState() (*AppState, error) {
	f, err := os.Open(stateFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var s AppState
	dec := json.NewDecoder(f)
	if err := dec.Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func saveState(s *AppState) error {
	// ensure directory exists
	if dir := filepath.Dir(stateFile); dir != "." && dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}

	tmp := stateFile + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, stateFile)
}

// ----------------------------
// Helpers & Validation
// ----------------------------

func prompt(reader *bufio.Reader, label string) string {
	fmt.Print(label)
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}

func atoiSafe(s string) (int, error) {
	i, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("enter a number")
	}
	return i, nil
}

func validateName(name string) bool { return len([]rune(strings.TrimSpace(name))) >= 2 }

func validateEmail(email string) bool { return emailRegex.MatchString(strings.TrimSpace(email)) }

// ----------------------------
// Core Operations
// ----------------------------

func bookTickets(s *AppState, first, last, email string, tickets int) (Booking, error) {
	if !validateName(first) || !validateName(last) {
		return Booking{}, errors.New("first and last name must have at least 2 characters")
	}
	if !validateEmail(email) {
		return Booking{}, errors.New("invalid email address")
	}
	if tickets <= 0 {
		return Booking{}, errors.New("tickets must be greater than 0")
	}
	if tickets > s.Conference.RemainingTickets {
		return Booking{}, fmt.Errorf("only %d tickets remaining", s.Conference.RemainingTickets)
	}

	id := atomic.AddUint64(&s.NextID, 1)
	b := Booking{
		ID:        id,
		FirstName: strings.Title(strings.ToLower(first)),
		LastName:  strings.Title(strings.ToLower(last)),
		Email:     strings.ToLower(email),
		Tickets:   tickets,
		BookedAt:  time.Now().Unix(),
	}
	s.Bookings = append(s.Bookings, b)
	s.Conference.RemainingTickets -= tickets

	// Fire-and-forget confirmation (simulated)
	go func(b Booking, confName string) {
		time.Sleep(2 * time.Second)
		fmt.Printf("\n✅ Confirmation sent to %s for %d ticket(s) to '%s' [Booking #%d].\n> ", b.Email, b.Tickets, confName, b.ID)
	}(b, s.Conference.Name)

	return b, nil
}

func listBookings(s *AppState) {
	if len(s.Bookings) == 0 {
		fmt.Println("No bookings yet.")
		return
	}
	fmt.Println("\nCurrent Bookings:")
	fmt.Println("ID\tName\t\tEmail\t\tTickets\tBooked At")
	for _, b := range s.Bookings {
		when := time.Unix(b.BookedAt, 0).Format(time.RFC1123)
		full := b.FirstName + " " + b.LastName
		fmt.Printf("%d\t%-16s\t%-24s\t%d\t%s\n", b.ID, full, b.Email, b.Tickets, when)
	}
}

func findBookingByID(s *AppState, id uint64) (*Booking, int) {
	for i, b := range s.Bookings {
		if b.ID == id {
			return &s.Bookings[i], i
		}
	}
	return nil, -1
}

func findBookingByEmail(s *AppState, email string) []Booking {
	email = strings.ToLower(strings.TrimSpace(email))
	var res []Booking
	for _, b := range s.Bookings {
		if b.Email == email {
			res = append(res, b)
		}
	}
	return res
}

func cancelBooking(s *AppState, id uint64) error {
	b, idx := findBookingByID(s, id)
	if b == nil {
		return errors.New("booking not found")
	}
	// restore tickets
	s.Conference.RemainingTickets += b.Tickets
	// remove from slice
	s.Bookings = append(s.Bookings[:idx], s.Bookings[idx+1:]...)
	return nil
}

func exportCSV(s *AppState, path string) error {
	if path == "" {
		path = "bookings.csv"
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	// header
	_ = w.Write([]string{"id", "first_name", "last_name", "email", "tickets", "booked_at"})
	for _, b := range s.Bookings {
		_ = w.Write([]string{
			strconv.FormatUint(b.ID, 10),
			b.FirstName,
			b.LastName,
			b.Email,
			strconv.Itoa(b.Tickets),
			time.Unix(b.BookedAt, 0).Format(time.RFC3339),
		})
	}
	return w.Error()
}

// ----------------------------
// UI (CLI)
// ----------------------------

func printHeader(conf Conference) {
	fmt.Println("====================================================")
	fmt.Printf("🎟️  %s — Ticketing CLI\n", conf.Name)
	fmt.Println("====================================================")
	fmt.Printf("Total Tickets: %d\tRemaining: %d\n", conf.TotalTickets, conf.RemainingTickets)
	fmt.Println()
}

func printMenu() {
	fmt.Println("Choose an option:")
	fmt.Println("  1) Book tickets")
	fmt.Println("  2) List bookings")
	fmt.Println("  3) Find booking (by ID or email)")
	fmt.Println("  4) Cancel booking")
	fmt.Println("  5) Show stats")
	fmt.Println("  6) Export CSV")
	fmt.Println("  0) Exit")
	fmt.Print("> ")
}

func showStats(s *AppState) {
	fmt.Println("\n— Stats —")
	fmt.Printf("Conference: %s\n", s.Conference.Name)
	fmt.Printf("Total tickets: %d\n", s.Conference.TotalTickets)
	fmt.Printf("Remaining tickets: %d\n", s.Conference.RemainingTickets)
	fmt.Printf("Total bookings: %d\n", len(s.Bookings))
	if len(s.Bookings) > 0 {
		// show first names as a quick view
		var names []string
		for _, b := range s.Bookings {
			names = append(names, b.FirstName)
		}
		fmt.Printf("Attendees (first names): %s\n", strings.Join(names, ", "))
	}
	fmt.Println()
}

func main() {
	reader := bufio.NewReader(os.Stdin)

	// Try load persisted state, otherwise initialize
	s, err := loadState()
	if err != nil {
		// Initialize
		confName := prompt(reader, "Enter conference name [Go Conference]: ")
		if confName == "" {
			confName = "Go Conference"
		}
		for {
			totalStr := prompt(reader, "Enter total number of tickets [100]: ")
			if totalStr == "" {
				totalStr = "100"
			}
			total, err := atoiSafe(totalStr)
			if err != nil || total <= 0 {
				fmt.Println("Please enter a positive number.")
				continue
			}
			s = &AppState{
				Conference: Conference{
					Name:             confName,
					TotalTickets:     total,
					RemainingTickets: total,
				},
				Bookings: []Booking{},
				NextID:   0,
			}
			_ = saveState(s)
			break
		}
	} else {
		fmt.Println("Loaded existing state from", stateFile)
	}

	for {
		printHeader(s.Conference)
		printMenu()
		choice := prompt(reader, "")

		switch choice {
		case "1":
			if s.Conference.RemainingTickets == 0 {
				fmt.Println("Sorry, the conference is sold out!\n")
				continue
			}
			first := prompt(reader, "First name: ")
			last := prompt(reader, "Last name: ")
			email := prompt(reader, "Email: ")
			max := s.Conference.RemainingTickets
			tkStr := prompt(reader, fmt.Sprintf("Number of tickets (max %d): ", max))
			tk, err := atoiSafe(tkStr)
			if err != nil {
				fmt.Println("Invalid number.\n")
				continue
			}
			b, err := bookTickets(s, first, last, email, tk)
			if err != nil {
				fmt.Println("Error:", err, "\n")
				continue
			}
			if err := saveState(s); err != nil {
				fmt.Println("Warning: couldn't save state:", err)
			}
			fmt.Printf("\n🎉 Booked! Booking ID: %d — %s %s for %d ticket(s).\n\n", b.ID, b.FirstName, b.LastName, b.Tickets)

		case "2":
			listBookings(s)
			fmt.Println()

		case "3":
			sub := prompt(reader, "Search by (1) ID or (2) Email? ")
			if sub == "1" {
				idStr := prompt(reader, "Enter booking ID: ")
				id64, err := strconv.ParseUint(strings.TrimSpace(idStr), 10, 64)
				if err != nil {
					fmt.Println("Invalid ID.\n")
					continue
				}
				b, _ := findBookingByID(s, id64)
				if b == nil {
					fmt.Println("Not found.\n")
				} else {
					when := time.Unix(b.BookedAt, 0).Format(time.RFC1123)
					fmt.Printf("Found: #%d — %s %s, %s, %d ticket(s), booked %s\n\n", b.ID, b.FirstName, b.LastName, b.Email, b.Tickets, when)
				}
			} else if sub == "2" {
				em := prompt(reader, "Enter email: ")
				matches := findBookingByEmail(s, em)
				if len(matches) == 0 {
					fmt.Println("No bookings for that email.\n")
				} else {
					for _, b := range matches {
						when := time.Unix(b.BookedAt, 0).Format(time.RFC1123)
						fmt.Printf("#%d — %s %s, %s, %d ticket(s), booked %s\n", b.ID, b.FirstName, b.LastName, b.Email, b.Tickets, when)
					}
					fmt.Println()
				}
			} else {
				fmt.Println("Unknown option.\n")
			}

		case "4":
			idStr := prompt(reader, "Enter booking ID to cancel: ")
			id64, err := strconv.ParseUint(strings.TrimSpace(idStr), 10, 64)
			if err != nil {
				fmt.Println("Invalid ID.\n")
				continue
			}
			if err := cancelBooking(s, id64); err != nil {
				fmt.Println("Error:", err, "\n")
				continue
			}
			if err := saveState(s); err != nil {
				fmt.Println("Warning: couldn't save state:", err)
			}
			fmt.Println("Booking cancelled. Tickets restored.\n")

		case "5":
			showStats(s)

		case "6":
			path := prompt(reader, "Export path [bookings.csv]: ")
			if path == "" {
				path = "bookings.csv"
			}
			if err := exportCSV(s, path); err != nil {
				fmt.Println("Export failed:", err)
			} else {
				fmt.Printf("Exported to %s\n\n", path)
			}

		case "0":
			fmt.Println("Goodbye!")
			return

		default:
			fmt.Println("Unknown choice. Try again.\n")
		}
	}
}
