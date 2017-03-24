package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	_ "github.com/lib/pq"
)

const (
	DB_USER     = "postgres"
	DB_PASSWORD = "postgres"
	DB_NAME     = "nusii_cloner_development"
)

var spin *spinner.Spinner

func main() {
	spin = spinner.New(spinner.CharSets[24], 100*time.Millisecond)
	// Verify that list subcommand is given
	if len(os.Args) < 2 {
		fmt.Println("Please provide a subcommand")
		os.Exit(1)
	}

	subCommand := os.Args[1]

	var err error

	// TODO return the errors in here instead of in the function
	switch subCommand {
	case "create_local_db":
		err = execCmdCreateLocalDb()
	case "drop_local_db":
		err = execCmdDropLocalDb()
	case "parse_database_backups":
		err = execCmdParseDatabaseBackups()
	case "fetch_and_import_backup":
		err = fetchAndImportBackup()
	default:
		flag.PrintDefaults()
		fmt.Printf("Subcommand %s is not in the list", subCommand)
		os.Exit(1)
	}

	if err != nil {
		log.Fatal(err)
	}

	return

}

type backup struct {
	id   string
	date time.Time
}

func (b backup) print() {
	fmt.Printf("%s %s\n", b.id, b.date)
}

const herokuTime = "2006-01-02 15:04:05"

func parseDatabaseBackups() ([]backup, error) {
	spin.Start()
	cmd := exec.Command("heroku", "pg:backups", "-a", "nusii2")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	var backups []backup
	scanner := bufio.NewScanner(&out)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 10 || fields[0] == "No" {
			continue
		}

		parsedDate, err := time.Parse(herokuTime, fields[1]+" "+fields[2])
		if err != nil {
			return nil, err
		}
		b := backup{id: fields[0], date: parsedDate}
		backups = append(backups, b)
	}
	spin.Stop()

	return backups, nil
}

func getBackupUrl(backup backup) (string, error) {
	cmd := exec.Command("heroku", "pg:backups", "public-url", backup.id, "-a", "nusii2")
	cmdResult, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	backupUrl := string(cmdResult[:])
	backupUrl = strings.Replace(backupUrl, "\n", "", -1)

	return backupUrl, nil
}

func restoreDump(fileName string) error {
	cmd := exec.Command("pg_restore", "-O", "-d", DB_NAME, "./"+fileName)

	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func fetchAndImportBackup() error {
	fmt.Print("Parsing database backups: ")

	spin.Start()
	backups, err := parseDatabaseBackups()
	if err != nil {
		return err
	}
	spin.Stop()
	fmt.Println("Done!")

	// by default the first one
	backup := backups[0]
	fmt.Printf("Using backup: %s %s\n", backup.id, backup.date)

	var backupUrl string
	backupUrl, err = getBackupUrl(backup)
	if err != nil {
		return err
	}

	// download the file and save it locally
	fileName := "dump.sql"

	_, err = os.Stat(fileName)

	// Delete file if exist
	if os.IsExist(err) {
		err = os.Remove(fileName)
		if err != nil {
			return err
		}
	}

	// Create file
	var output *os.File
	output, err = os.Create(fileName)
	defer output.Close()
	if err != nil {
		return err
	}

	// Download dump
	fmt.Print("Downloading backup: ")
	spin.Start()
	var response *http.Response
	response, err = http.Get(backupUrl)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	_, err = io.Copy(output, response.Body)
	if err != nil {
		return err
	}
	spin.Stop()
	fmt.Println("Done!")

	fmt.Print("Restoring Dump: ")
	spin.Start()
	err = restoreDump(fileName)
	if err != nil {
		return err
	}
	spin.Stop()
	fmt.Println("Done!")

	// delete dump file
	err = os.Remove(fileName)
	if err != nil {
		return err
	}
	fmt.Println("Dump file deleted")
	return nil
}

func execCmdCreateLocalDb() error {
	dbinfo := fmt.Sprintf("user=%s password=%s sslmode=disable", DB_USER, DB_PASSWORD)
	db, err := sql.Open("postgres", dbinfo)

	if err != nil {
		return err
	}

	_, err = db.Query("CREATE DATABASE nusii_cloner_development")
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			fmt.Println("database nusii_cloner_development already exists")
			os.Exit(0)
		}
		return err
	}

	fmt.Println("database nusii_cloner_development is created")

	return nil
}

func execCmdDropLocalDb() error {
	dbinfo := fmt.Sprintf("user=%s password=%s sslmode=disable", DB_USER, DB_PASSWORD)

	db, err := sql.Open("postgres", dbinfo)
	if err != nil {
		return err
	}

	_, err = db.Query("DROP DATABASE nusii_cloner_development")
	if err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			fmt.Println("database nusii_cloner_development does not exists")
			os.Exit(0)
		}
		return err
	}

	fmt.Println("database nusii_cloner_development is deleted")

	return nil
}

func execCmdParseDatabaseBackups() error {
	backups, err := parseDatabaseBackups()

	if err != nil {
		return err
	}

	for _, b := range backups {
		b.print()
	}

	return nil
}

func temp() {
	subCommand := os.Args[1]
	doAwesomeCommand := flag.NewFlagSet("do_awesome", flag.ExitOnError)

	doAwesomeProposalId := doAwesomeCommand.String("proposal_id", "", "Proposal Id (Required)")
	doAwesomeHelp := doAwesomeCommand.Bool("h", false, "Awesome Help")
	// doAwesomeAccountId := doAwesomeCommand.String("account_id", "", "Account Id")

	//fmt.Printf("proposalId: %s, accountId: %s", *proposalId, *accountId)

	// Verify that list subcommand is given
	if len(os.Args) < 2 {
		fmt.Println("Please provide a subcommand")
		os.Exit(1)
	}

	switch subCommand {
	case "do_awesome":
		doAwesomeCommand.Parse(os.Args[2:])
	default:
		flag.PrintDefaults()
		fmt.Printf("Subcommand %s is not in the list", subCommand)
		os.Exit(1)
	}

	if doAwesomeCommand.Parsed() {
		if *doAwesomeHelp == true {
			os.Exit(0)
		} else if *doAwesomeProposalId == "" {
			doAwesomeCommand.PrintDefaults()
			os.Exit(1)
		}

		fmt.Printf("doing awesome command with proposal id: %s", *doAwesomeProposalId)
	}

}
