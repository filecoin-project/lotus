package lpseal

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestUrlPieceReader_Read tests various scenarios of reading data from UrlPieceReader
func TestUrlPieceReader_Read(t *testing.T) {
	// Create a test server
	testData := "This is a test string."
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, testData)
	}))
	defer ts.Close()

	tests := []struct {
		name        string
		rawSize     int64
		expected    string
		expectError bool
		expectEOF   bool
	}{
		{"ReadExact", int64(len(testData)), testData, false, true},
		{"ReadLess", 10, testData[:10], false, false},
		{"ReadMore", int64(len(testData)) + 10, "", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := UrlPieceReader{
				Url:     ts.URL,
				RawSize: tt.rawSize,
			}
			buffer, err := io.ReadAll(&reader)
			if err != nil {
				if (err != io.EOF && !tt.expectError) || (err == io.EOF && !tt.expectEOF) {
					t.Errorf("Read() error = %v, expectError %v, expectEOF %v", err, tt.expectError, tt.expectEOF)
				}
			} else {
				if got := string(buffer); got != tt.expected {
					t.Errorf("Read() got = %v, expected %v", got, tt.expected)
				}
			}
		})
	}
}

// TestUrlPieceReader_Read_Error tests the error handling of UrlPieceReader
func TestUrlPieceReader_Read_Error(t *testing.T) {
	// Simulate a server that returns an error
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "error", http.StatusInternalServerError)
	}))
	defer ts.Close()

	reader := UrlPieceReader{
		Url:     ts.URL,
		RawSize: 100,
	}
	buffer := make([]byte, 200)

	_, err := reader.Read(buffer)
	if err == nil {
		t.Errorf("Expected an error, but got nil")
	}
}
