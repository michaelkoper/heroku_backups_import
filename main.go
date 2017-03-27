package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	kingpin "gopkg.in/alecthomas/kingpin.v2"

	"github.com/briandowns/spinner"
	_ "github.com/lib/pq"
)

const (
	DB_NAME = "heroku_backups_import"
)

var (
	dbName    string = DB_NAME
	herokuApp string
)

var spin *spinner.Spinner

var (
	app            = kingpin.New("heroku_backups_import", "A command-line interface tool to easily import heroku backups into a local database")
	dbNameFlag     = app.Flag("db", "Name of database").Short('d').String()
	herokuAppFlag  = app.Flag("app", "Name of heroku app").Short('a').Required().String()
	backupDateFlag = app.Flag("date", "Date of heroku backup").String()
	backupIdFlag   = app.Flag("backup-id", "ID of a heroku backup").String()

	fetchAndImportCmd = app.Command("import", "Fetch and Import Heroku backup into a local database")
	showBackupsCmd    = app.Command("show_backups", "Show available Heroku backups")
)

func main() {
	spin = spinner.New(spinner.CharSets[24], 100*time.Millisecond)

	var err error

	kingpin.Version("0.0.1")
	cmd := kingpin.MustParse(app.Parse(os.Args[1:]))

	if len(*dbNameFlag) > 0 {
		dbName = *dbNameFlag
	}
	if len(*herokuAppFlag) > 0 {
		herokuApp = *herokuAppFlag
	}

	switch cmd {
	case fetchAndImportCmd.FullCommand():
		err = fetchAndImportBackup()
	case showBackupsCmd.FullCommand():
		err = execCmdParseDatabaseBackups()
	}

	if err != nil {
		log.Fatal(err)
	}

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
	cmd := exec.Command("heroku", "pg:backups", "-a", herokuApp)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("Heroku pg:backups failed: %s. %v", errOut.String(), err)
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
	cmd := exec.Command("heroku", "pg:backups", "public-url", backup.id, "-a", herokuApp)
	cmdResult, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	backupUrl := string(cmdResult[:])
	backupUrl = strings.Replace(backupUrl, "\n", "", -1)

	return backupUrl, nil
}

func restoreDump(fileName string) error {
	cmd := exec.Command("pg_restore", "-O", "-c", "-d", dbName, "./"+fileName)

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
	var backup backup
	if len(*backupIdFlag) > 0 {
		for index, value := range backups {
			if value.id == *backupIdFlag {
				backup = backups[index]
				break
			}
		}
	}

	if backup.id == "" && len(*backupDateFlag) > 0 {
		for index, value := range backups {
			if value.date.Format("2006-01-02") == *backupDateFlag {
				backup = backups[index]
				break
			}
		}
	}
	if backup.id == "" {
		if len(*backupIdFlag) > 0 {
			fmt.Println("Couldn't find backup with id:", *backupIdFlag)
		}
		if len(*backupDateFlag) > 0 {
			fmt.Println("Couldn't find backup with date:", *backupDateFlag)
		}
		backup = backups[0]
	}
	fmt.Printf("Using backup: %s %s\n", backup.id, backup.date)

	backupUrl, err := getBackupUrl(backup)
	if err != nil {
		return err
	}

	// download the file and save it locally
	fileName := "dump.sql"

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
