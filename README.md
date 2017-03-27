# heroku_backups_import
```
usage: heroku_backups_import --app=APP [<flags>] <command> [<args> ...]

A command-line interface tool to easily import heroku backups into a local database

Flags:
      --help                 Show context-sensitive help (also try --help-long and --help-man).
  -d, --db=DB                Name of database
  -a, --app=APP              Name of heroku app
      --date=DATE            Date of heroku backup
      --backup-id=BACKUP-ID  ID of a heroku backup

Commands:
  help [<command>...]
    Show help.

  import
    Fetch and Import Heroku backup into a local database

  show_backups
    Show available Heroku backups
```
