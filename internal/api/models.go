package api

type Venue struct {
	ID        *int    `json:"id,omitempty"`
	Name      string  `json:"name"`
	Address   string  `json:"address"`
	Phone     *string `json:"phone,omitempty"`
	CreatedAt *string `json:"createdAt,omitempty"`
	UpdatedAt *string `json:"updatedAt,omitempty"`
}

type EventSession struct {
	ID                int     `json:"id"`
	StartAt           string  `json:"startAt"`
	EndAt             *string `json:"endAt,omitempty"`
	IsOnline          bool    `json:"is_online"`
	TicketMinPrice    *int    `json:"ticketMinPrice,omitempty"`
	TicketMaxPrice    *int    `json:"ticketMaxPrice,omitempty"`
	Currency          *string `json:"currency,omitempty"`
	TicketURL         *string `json:"ticketUrl,omitempty"`
	TicketServiceName *string `json:"ticketServiceName,omitempty"`
	Status            string  `json:"status"`
	CreatedAt         string  `json:"createdAt"`
	UpdatedAt         string  `json:"updatedAt"`
	Venue             *Venue  `json:"venue,omitempty"`
}

type Event struct {
	ID                   int            `json:"id"`
	Title                string         `json:"title"`
	Subtitle             string         `json:"subtitle,omitempty"`
	ShortDescription     string         `json:"shortDescription"`
	FullDescription      string         `json:"fullDescription"`
	Category             string         `json:"category"`
	AgeRestriction       *int           `json:"ageRestriction,omitempty"`
	CoverImageURL        string         `json:"coverImageUrl,omitempty"`
	OrganizerName        string         `json:"organizerName"`
	OrganizerDescription string         `json:"organizerDescription"`
	Status               string         `json:"status"`
	CreatedAt            string         `json:"createdAt"`
	UpdatedAt            string         `json:"updatedAt"`
	EventSessions        []EventSession `json:"eventSessions"`
}
