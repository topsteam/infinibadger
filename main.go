package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds"
)

type client struct {
	rds      *rds.RDS
	instance string
	lf       *os.File
}

func newClient(cfg *config, f *os.File) *client {
	creds := credentials.NewStaticCredentials(cfg.AWSAccessKey, cfg.AWSSecretKey, "")
	sess := session.Must(session.NewSession(&aws.Config{
		Credentials: creds,
		Region:      aws.String(cfg.AWSRegion),
	}))

	return &client{
		rds:      rds.New(sess),
		instance: cfg.Instance,
		lf:       f,
	}
}

// state of the download
type state struct {
	LastWritten int64
	LogFileName string
	Marker      string
	Size        int64
}

func (c *client) downloadLogs(s *state) error {
	params := &rds.DescribeDBLogFilesInput{
		DBInstanceIdentifier: aws.String(c.instance),
		FileLastWritten:      aws.Int64(s.LastWritten),
	}
	resp, err := c.rds.DescribeDBLogFiles(params)
	if err != nil {
		return err
	}

	for _, f := range resp.DescribeDBLogFiles {
		if err := c.download(f, s); err != nil {
			return err
		}
		// AWS api will return last file if it was changed since last describe
		s.LastWritten = aws.Int64Value(f.LastWritten)
	}

	return nil
}

func (c *client) download(f *rds.DescribeDBLogFilesDetails, s *state) error {
	fname := aws.StringValue(f.LogFileName)
	// Reset state for a new file
	if s.LogFileName != fname {
		s.LogFileName = fname
		s.Marker = "0"
		s.Size = 0
	}
	// start size
	size := s.Size

	for {
		params := &rds.DownloadDBLogFilePortionInput{
			DBInstanceIdentifier: aws.String(c.instance),
			LogFileName:          f.LogFileName,
			Marker:               aws.String(s.Marker),
		}
		resp, err := c.rds.DownloadDBLogFilePortion(params)
		if err != nil {
			return err
		}

		if resp.LogFileData != nil {
			// AWS api truncates some strings and adds suffix
			// We cut suffix down and adjust marker
			a := aws.StringValue(resp.LogFileData)
			if strings.HasSuffix(a, " [Your log message was truncated]\n") {
				a = a[:len(a)-35]
			}

			n, err := c.lf.WriteString(a)
			if err != nil {
				return err
			}
			s.Size += int64(n)

			// resp.AdditionalDataPending can return false even there is data left
			if aws.Int64Value(f.Size) > s.Size {
				mm := strings.Split(aws.StringValue(resp.Marker), ":")
				s.Marker = fmt.Sprintf("%s:%d", mm[0], s.Size)
				continue
			}
		}
		break
	}
	log.Printf("downloaded file=%s size=%d", fname, s.Size-size)
	return nil
}

func badger(cfg *config, fname string) error {
	outdir, err := filepath.Abs(cfg.Outdir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outdir, 0755); err != nil {
		return err
	}
	path, err := exec.LookPath("pgbadger")
	if err != nil {
		return err
	}

	args := []string{
		path,
		"--incremental",
		"--anonymize",
		"--start-monday",
		fmt.Sprintf("--prefix '%s'", cfg.Prefix),
		fmt.Sprintf("--retention %s", cfg.Retention),
		fmt.Sprintf("--outdir %s", outdir),
		fname,
	}

	out, err := exec.Command("/bin/sh", "-c", strings.Join(args, " ")).CombinedOutput()
	for _, s := range strings.Split(strings.Trim(string(out), "\n"), "\n") {
		log.Print(s)
	}
	return err
}

func serve(cfg *config) {
	log.Printf("starting http server on %s", cfg.ListenAddress)
	log.Fatal(http.ListenAndServe(cfg.ListenAddress, http.FileServer(http.Dir(cfg.Outdir))))
}

type config struct {
	fs *flag.FlagSet

	ListenAddress    string
	DownloadInterval time.Duration
	PrintVersion     bool
	// AWS
	AWSAccessKey string
	AWSSecretKey string
	AWSRegion    string
	Instance     string
	// pgBadger
	Outdir    string
	Prefix    string
	Retention string
}

var version = "0.1"

func main() {
	// logging setup
	log.SetFlags(log.Ldate | log.Lmicroseconds)

	cfg := &config{fs: flag.NewFlagSet("infinibadger", flag.ContinueOnError)}

	cfg.fs.StringVar(
		&cfg.ListenAddress, "listen-address", ":8080",
		"Address to listen on for the HTTP Server",
	)
	cfg.fs.DurationVar(
		&cfg.DownloadInterval, "download-interval", 15*time.Minute,
		"How often to query for new files",
	)
	cfg.fs.BoolVar(
		&cfg.PrintVersion, "version", false,
		"Print the version and exit",
	)

	cfg.fs.StringVar(&cfg.AWSAccessKey, "aws-access-key", "", "AWS access key")
	cfg.fs.StringVar(&cfg.AWSSecretKey, "aws-secret-key", "", "AWS secret key")
	cfg.fs.StringVar(&cfg.AWSRegion, "aws-region", "us-east-1", "AWS geographical region")
	cfg.fs.StringVar(&cfg.Instance, "instance", "", "RDS Instance")

	cfg.fs.StringVar(
		&cfg.Outdir, "pgb-outdir", "outdir",
		"pgBadger output directory",
	)
	cfg.fs.StringVar(
		&cfg.Retention, "pgb-retention", "4",
		"Number of weeks to keep reports",
	)
	cfg.fs.StringVar(
		&cfg.Prefix, "pgb-prefix", "%t:%r:%u@%d:[%p]:",
		"log_line_prefix as defined in your postgresql.conf",
	)

	if err := cfg.fs.Parse(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	if cfg.PrintVersion {
		fmt.Println("infinibadger version", version)
		fmt.Println("go version", runtime.Version())
		os.Exit(0)
	}

	go serve(cfg)

	state := &state{}
	timer := time.NewTimer(0)
	for {
		<-timer.C

		// func so we can defer
		func() {
			log.Printf("starting download state=%#v", state)

			lf, err := ioutil.TempFile("", "infinibadger")
			if err != nil {
				log.Printf("failed to create temp file err=%s", err)
				return
			}
			log.Printf("created temp file %s", lf.Name())
			defer os.Remove(lf.Name())

			if err := newClient(cfg, lf).downloadLogs(state); err != nil {
				log.Printf("failed to download logs err=%s", err)
				return
			}

			lf.Close()

			if err := badger(cfg, lf.Name()); err != nil {
				log.Printf("failed to run pgbadger err=%s", err)
				return
			}
		}()
		timer.Reset(cfg.DownloadInterval)
	}
}
