package postgres

import (
	"database/sql"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/go-pg/pg"
	jsoniter "github.com/json-iterator/go"
	_ "github.com/lib/pq"
	"github.com/praetorian-inc/fingerprintx/pkg/plugins"
	postgres "github.com/praetorian-inc/fingerprintx/pkg/plugins/services/postgresql"
)

// Client is a client for Postgres database.
//
// Internally client uses go-pg/pg driver.
type Client struct{}

// IsPostgres checks if the given host and port are running Postgres database.
//
// If connection is successful, it returns true.
// If connection is unsuccessful, it returns false and error.
func (c *Client) IsPostgres(host string, port int) (bool, error) {
	timeout := 10 * time.Second

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), timeout)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(timeout))

	plugin := &postgres.POSTGRESPlugin{}
	service, err := plugin.Run(conn, timeout, plugins.Target{Host: host})
	if err != nil {
		return false, err
	}
	if service == nil {
		return false, nil
	}
	return true, nil
}

// Connect connects to Postgres database using given credentials.
//
// If connection is successful, it returns true.
// If connection is unsuccessful, it returns false and error.
//
// The connection is closed after the function returns.
func (c *Client) Connect(host string, port int, username, password string) (bool, error) {
	return connect(host, port, username, password, "postgres")
}

// ExecuteQuery connects to Postgres database using given credentials and database name.
// and executes a query on the db.
func (c *Client) ExecuteQuery(host string, port int, username, password, dbName, query string) (string, error) {
	target := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	connStr := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", username, password, target, dbName)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return "", err
	}

	rows, err := db.Query(query)
	if err != nil {
		return "", err
	}
	resp, err := unmarshalSQLRows(rows)
	if err != nil {
		return "", err
	}
	return string(resp), nil
}

func unmarshalSQLRows(rows *sql.Rows) ([]byte, error) {
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}

	count := len(columnTypes)
	finalRows := []interface{}{}

	for rows.Next() {

		scanArgs := make([]interface{}, count)

		for i, v := range columnTypes {

			switch v.DatabaseTypeName() {
			case "VARCHAR", "TEXT", "UUID", "TIMESTAMP":
				scanArgs[i] = new(sql.NullString)
				break
			case "BOOL":
				scanArgs[i] = new(sql.NullBool)
				break
			case "INT4":
				scanArgs[i] = new(sql.NullInt64)
				break
			default:
				scanArgs[i] = new(sql.NullString)
			}
		}

		err := rows.Scan(scanArgs...)

		if err != nil {
			return nil, err
		}

		masterData := map[string]interface{}{}

		for i, v := range columnTypes {

			if z, ok := (scanArgs[i]).(*sql.NullBool); ok {
				masterData[v.Name()] = z.Bool
				continue
			}

			if z, ok := (scanArgs[i]).(*sql.NullString); ok {
				masterData[v.Name()] = z.String
				continue
			}

			if z, ok := (scanArgs[i]).(*sql.NullInt64); ok {
				masterData[v.Name()] = z.Int64
				continue
			}

			if z, ok := (scanArgs[i]).(*sql.NullFloat64); ok {
				masterData[v.Name()] = z.Float64
				continue
			}

			if z, ok := (scanArgs[i]).(*sql.NullInt32); ok {
				masterData[v.Name()] = z.Int32
				continue
			}

			masterData[v.Name()] = scanArgs[i]
		}

		finalRows = append(finalRows, masterData)
	}
	return jsoniter.Marshal(finalRows)
}

// ConnectWithDB connects to Postgres database using given credentials and database name.
//
// If connection is successful, it returns true.
// If connection is unsuccessful, it returns false and error.
//
// The connection is closed after the function returns.
func (c *Client) ConnectWithDB(host string, port int, username, password, dbName string) (bool, error) {
	return connect(host, port, username, password, dbName)
}

func connect(host string, port int, username, password, dbName string) (bool, error) {
	if host == "" || port <= 0 {
		return false, fmt.Errorf("invalid host or port")
	}
	target := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	db := pg.Connect(&pg.Options{
		Addr:     target,
		User:     username,
		Password: password,
		Database: dbName,
	})
	_, err := db.Exec("select 1")
	if err != nil {
		switch true {
		case strings.Contains(err.Error(), "connect: connection refused"):
			fallthrough
		case strings.Contains(err.Error(), "no pg_hba.conf entry for host"):
			fallthrough
		case strings.Contains(err.Error(), "network unreachable"):
			fallthrough
		case strings.Contains(err.Error(), "reset"):
			fallthrough
		case strings.Contains(err.Error(), "i/o timeout"):
			return false, err
		}
		return false, nil
	}
	return true, nil
}
