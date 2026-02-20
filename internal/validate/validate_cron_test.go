package validate

import "testing"

func TestCronExpression(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"every_minute", "* * * * *", false},
		{"every_hour", "0 * * * *", false},
		{"every_day_noon", "0 12 * * *", false},
		{"weekdays_9am", "0 9 * * 1-5", false},
		{"first_of_month", "0 0 1 * *", false},
		{"descriptor_hourly", "@hourly", false},
		{"descriptor_daily", "@daily", false},
		{"descriptor_weekly", "@weekly", false},
		{"descriptor_monthly", "@monthly", false},
		{"descriptor_yearly", "@yearly", false},
		{"descriptor_annually", "@annually", false},
		{"every_5_minutes", "*/5 * * * *", false},
		{"complex", "15 2,14 1,15 * 1-5", false},

		{"empty", "", true},
		{"too_few_fields", "* * *", true},
		{"too_many_fields", "* * * * * *", true},
		{"invalid_minute", "60 * * * *", true},
		{"invalid_hour", "0 25 * * *", true},
		{"invalid_day", "0 0 32 * *", true},
		{"invalid_month", "0 0 * 13 *", true},
		{"invalid_dow", "0 0 * * 8", true},
		{"garbage", "not a cron", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := CronExpression(tt.expr)
			if tt.wantErr && err == nil {
				t.Errorf("CronExpression(%q) = nil, want error", tt.expr)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("CronExpression(%q) = %v, want nil", tt.expr, err)
			}
		})
	}
}

func TestParseCron(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"valid_every_minute", "* * * * *", false},
		{"valid_descriptor", "@hourly", false},
		{"invalid_expression", "bad", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sched, err := ParseCron(tt.expr)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseCron(%q) = nil error, want error", tt.expr)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseCron(%q) error = %v", tt.expr, err)
				return
			}
			if sched == nil {
				t.Errorf("ParseCron(%q) returned nil schedule", tt.expr)
			}
		})
	}
}

func TestTimezone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tz      string
		wantErr bool
	}{
		{"utc", "UTC", false},
		{"us_eastern", "America/New_York", false},
		{"us_pacific", "America/Los_Angeles", false},
		{"europe_london", "Europe/London", false},
		{"asia_tokyo", "Asia/Tokyo", false},
		{"local", "Local", false},

		{"empty_is_utc", "", false},
		{"invalid", "Not/A/Timezone", true},
		{"garbage", "garbage", true},
		{"partial", "America/", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := Timezone(tt.tz)
			if tt.wantErr && err == nil {
				t.Errorf("Timezone(%q) = nil, want error", tt.tz)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Timezone(%q) = %v, want nil", tt.tz, err)
			}
		})
	}
}
