package models

import (
	"github.com/google/uuid"
	"time"
)

type DriverProfile struct {
	UserID       uuid.UUID `gorm:"primaryKey" json:"userId"`
	Phone        string    `json:"phone"`
	VehiclePlate string    `json:"vehiclePlate"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
