# Infinibadger

[![Go Report Card](https://goreportcard.com/badge/github.com/topsteam/infinibadger)](https://goreportcard.com/report/github.com/topsteam/infinibadger)

Exports logs from AWS RDS, feeds them to pgBadger and serves html report via HTTP.
Handles AWS log truncation.


## Usage

```
./infinibadger --help
Usage of infinibadger:
  -aws-access-key string
    	AWS access key
  -aws-region string
    	AWS geographical region (default "us-east-1")
  -aws-secret-key string
    	AWS secret key
  -download-interval duration
    	How often to query for new files (default 15m0s)
  -instance string
    	RDS Instance
  -listen-address string
    	Address to listen on for the HTTP Server (default ":8080")
  -pgb-outdir string
    	pgBadger output directory (default "outdir")
  -pgb-prefix string
    	log_line_prefix as defined in your postgresql.conf (default "%t:%r:%u@%d:[%p]:")
  -pgb-retention string
    	Number of weeks to keep reports (default "4")
  -version
    	Print the version and exit
```