package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	fmt.Println("FlightStrips Recording Tool")
	fmt.Println("============================")
	fmt.Println()
	fmt.Println("This tool helps you record EuroScope WebSocket sessions for replay testing.")
	fmt.Println()
	fmt.Println("To record a session:")
	fmt.Println("1. Set environment variables:")
	fmt.Println("   - TEST_MODE=true")
	fmt.Println("   - RECORD_MODE=true")
	fmt.Println("   - RECORDING_PATH=recordings (optional, defaults to 'recordings')")
	fmt.Println("2. Start the FlightStrips backend server")
	fmt.Println("3. Connect with EuroScope")
	fmt.Println("4. The session will be automatically recorded")
	fmt.Println("5. When done, stop the server or disconnect")
	fmt.Println()
	fmt.Println("The recorded session will be saved as a JSON file in the recordings directory.")
	fmt.Println()
	
	reader := bufio.NewReader(os.Stdin)
	
	fmt.Print("Would you like to generate a sample .env file for recording? (y/n): ")
	response, err := reader.ReadString('\n')
	if err != nil {
		log.Fatal(err)
	}
	
	response = strings.TrimSpace(strings.ToLower(response))
	if response == "y" || response == "yes" {
		createEnvFile()
	} else {
		fmt.Println("Skipping .env file creation.")
	}
	
	fmt.Println()
	fmt.Println("Happy recording!")
}

func createEnvFile() {
	envContent := `# FlightStrips Recording Configuration
TEST_MODE=true
RECORD_MODE=true
RECORDING_PATH=recordings

# Database configuration
DATABASE_CONNECTIONSTRING=postgresql://fs:fs_password@localhost:5432/fsdb?sslmode=disable

# OIDC configuration (not validated in TEST_MODE)
OIDC_AUTHORITY=https://auth.flightstrips.dk/.well-known/jwks.json
OIDC_SIGNING_ALGO=RS256
`

	filename := ".env.recording"
	err := os.WriteFile(filename, []byte(envContent), 0644)
	if err != nil {
		log.Fatalf("Failed to create .env.recording file: %v", err)
	}
	
	fmt.Printf("\nCreated %s\n", filename)
	fmt.Println("You can use it by running:")
	fmt.Println("  cp .env.recording .env")
	fmt.Println("  go run cmd/server/main.go")
}