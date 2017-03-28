package main

import (
	"bufio"
	"bytes"
	"database/sql"
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
	DB_USER     = "postgres"
	DB_PASSWORD = "postgres"
	DB_NAME     = "heroku_backups_import"
)

var (
	dbName     string = DB_NAME
	dbUser     string = DB_USER
	dbPassword string = DB_PASSWORD
	herokuApp  string
)

var spin *spinner.Spinner

var (
	app            = kingpin.New("heroku_backups_import", "A command-line interface tool to easily import heroku backups into a local database")
	dbNameFlag     = app.Flag("db", "Name of database").Short('d').String()
	dbUserFlag     = app.Flag("db-user", "Username of database").String()
	dbPasswordFlag = app.Flag("db-password", "Password of database").String()
	backupDateFlag = app.Flag("date", "Date of heroku backup").String()
	backupIdFlag   = app.Flag("backup-id", "ID of a heroku backup").String()

	fetchAndImportCmd              = app.Command("import", "Fetch and Import Heroku backup into a local database")
	fetchAndImportCmdHerokuAppFlag = fetchAndImportCmd.Flag("app", "Name of heroku app").Short('a').Required().String()

	showBackupsCmd              = app.Command("show_backups", "Show available Heroku backups")
	showBackupsCmdHerokuAppFlag = showBackupsCmd.Flag("app", "Name of heroku app").Short('a').Required().String()

	flushDbCmd          = app.Command("flush_db", "Empty all tables")
	flushDbCmdForceFlag = flushDbCmd.Flag("force", "Force empty all tables").Short('f').Required().Bool()
)

func main() {
	spin = spinner.New(spinner.CharSets[24], 100*time.Millisecond)

	var err error

	kingpin.Version("0.0.1")
	cmd := kingpin.MustParse(app.Parse(os.Args[1:]))

	if len(*dbNameFlag) > 0 {
		dbName = *dbNameFlag
	}
	if len(*dbUserFlag) > 0 {
		dbUser = *dbUserFlag
	}
	if len(*dbPasswordFlag) > 0 {
		dbPassword = *dbPasswordFlag
	}
	if len(*fetchAndImportCmdHerokuAppFlag) > 0 {
		herokuApp = *fetchAndImportCmdHerokuAppFlag
	}
	if len(*showBackupsCmdHerokuAppFlag) > 0 {
		herokuApp = *showBackupsCmdHerokuAppFlag
	}

	switch cmd {
	case fetchAndImportCmd.FullCommand():
		err = fetchAndImportBackup()
	case showBackupsCmd.FullCommand():
		err = execCmdParseDatabaseBackups()
	case flushDbCmd.FullCommand():
		err = execCmdFlushDatabase()
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
	var errOut bytes.Buffer
	cmd.Stderr = &errOut

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("pg_restore failed: %s. %v", errOut.String(), err)
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

func execCmdFlushDatabase() error {
	db, err := openDB()
	defer db.Close()

	if err != nil {
		return err
	}

	var tableName string
	var tableNames []string

	var rows *sql.Rows
	rows, err = db.Query("SELECT table_name FROM information_schema.tables WHERE table_schema = 'public';")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		err := rows.Scan(&tableName)
		if err != nil {
			return err
		}
		tableNames = append(tableNames, tableName)
	}

	err = rows.Err()
	if err != nil {
		return err
	}

	transaction, err := db.Begin()
	if err != nil {
		return err
	}

	defer func() {
		_ = transaction.Rollback()
	}()

	for _, value := range tableNames {
		_, err := transaction.Exec("TRUNCATE " + value + " CASCADE")
		if err != nil {
			fmt.Println("err", err)
			return err
		}
	}

	err = transaction.Commit()

	if err != nil {
		return err
	}

	fmt.Println("Database is flushed")
	return nil
}

func openDB() (*sql.DB, error) {
	dbinfo := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", dbUser, dbPassword, dbName)
	return sql.Open("postgres", dbinfo)
}
