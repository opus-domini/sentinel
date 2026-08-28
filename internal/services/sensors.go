package services

// SensorMetrics groups physical host telemetry exposed by the operating system.
// Empty categories are encoded as empty arrays so callers can distinguish an
// available metrics response from an unavailable sensor class.
type SensorMetrics struct {
	Temperatures []TemperatureSensor `json:"temperatures"`
	Fans         []FanSensor         `json:"fans"`
	Power        []PowerSensor       `json:"power"`
}

// TemperatureSensor is one hardware temperature reading in degrees Celsius.
type TemperatureSensor struct {
	ID              string   `json:"id"`
	Label           string   `json:"label"`
	Source          string   `json:"source"`
	Celsius         float64  `json:"celsius"`
	MaxCelsius      *float64 `json:"maxCelsius,omitempty"`
	CriticalCelsius *float64 `json:"criticalCelsius,omitempty"`
	Alarm           *bool    `json:"alarm,omitempty"`
}

// FanSensor is one hardware fan tachometer reading in revolutions per minute.
type FanSensor struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Source string `json:"source"`
	RPM    int64  `json:"rpm"`
	MinRPM *int64 `json:"minRpm,omitempty"`
	MaxRPM *int64 `json:"maxRpm,omitempty"`
	Alarm  *bool  `json:"alarm,omitempty"`
}

// PowerSensor is one instantaneous or interval-derived power reading in watts.
type PowerSensor struct {
	ID            string   `json:"id"`
	Label         string   `json:"label"`
	Source        string   `json:"source"`
	Watts         float64  `json:"watts"`
	MaxWatts      *float64 `json:"maxWatts,omitempty"`
	CriticalWatts *float64 `json:"criticalWatts,omitempty"`
	CapWatts      *float64 `json:"capWatts,omitempty"`
	Alarm         *bool    `json:"alarm,omitempty"`
}

func emptySensorMetrics() SensorMetrics {
	return SensorMetrics{
		Temperatures: make([]TemperatureSensor, 0),
		Fans:         make([]FanSensor, 0),
		Power:        make([]PowerSensor, 0),
	}
}
