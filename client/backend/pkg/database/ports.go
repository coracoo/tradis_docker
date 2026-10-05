package database

import (
	"database/sql"
	"strings"
)

type PortRange struct {
	Start    int    `json:"start"`
	End      int    `json:"end"`
	Protocol string `json:"protocol"`
}

type PortNoteKey struct {
	Port     int
	Type     string
	Protocol string
}

func SavePortRangeTx(tx *sql.Tx, start, end int, protocol string) error {
	_, err := tx.Exec(`DELETE FROM port_settings`)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO port_settings (range_start, range_end, protocol, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)`, start, end, protocol)
	return err
}

func GetPortRange() (*PortRange, error) {
	var pr PortRange
	pr.Start = 0
	pr.End = 65535
	pr.Protocol = "TCP+UDP"
	row := GetDB().QueryRow(`SELECT range_start, range_end, protocol FROM port_settings ORDER BY updated_at DESC LIMIT 1`)
	var start, end int
	var proto string
	err := row.Scan(&start, &end, &proto)
	if err == sql.ErrNoRows {
		return &pr, nil
	}
	if err != nil {
		return nil, err
	}
	pr.Start = start
	pr.End = end
	pr.Protocol = proto
	return &pr, nil
}

func SavePortNoteTx(tx *sql.Tx, port int, t, protocol, note string) error {
	t = strings.ToLower(strings.TrimSpace(t))
	if t != "" {
		t = strings.ToUpper(t[:1]) + t[1:]
	}
	protocol = strings.ToUpper(strings.TrimSpace(protocol))
	_, err := tx.Exec(`INSERT INTO port_notes (port, type, protocol, note, updated_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
        ON CONFLICT(port, type, protocol) DO UPDATE SET note=excluded.note, updated_at=CURRENT_TIMESTAMP`, port, t, protocol, note)
	return err
}

func GetAllPortNotes() (map[PortNoteKey]string, error) {
	rows, err := GetDB().Query(`SELECT port, type, protocol, note FROM port_notes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := make(map[PortNoteKey]string)
	for rows.Next() {
		var port int
		var t, proto, note string
		if err := rows.Scan(&port, &t, &proto, &note); err != nil {
			return nil, err
		}
		res[PortNoteKey{Port: port, Type: t, Protocol: proto}] = note
	}
	return res, nil
}

func ReservePortsTx(tx *sql.Tx, ports []int, reservedBy, protocol, t string) error {
	if len(ports) == 0 {
		return nil
	}
	for _, p := range ports {
		_, err := tx.Exec(`INSERT OR IGNORE INTO port_reservations (port, reserved_by, protocol, type, reserved_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`, p, reservedBy, protocol, t)
		if err != nil {
			return err
		}
	}
	return nil
}

func GetReservedPorts() (map[int]bool, error) {
	rows, err := GetDB().Query(`SELECT port FROM port_reservations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := map[int]bool{}
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		res[p] = true
	}
	return res, nil
}

func DeleteReservedPortsByPortsTx(tx *sql.Tx, ports []int) error {
	if len(ports) == 0 {
		return nil
	}
	for _, p := range ports {
		if _, err := tx.Exec(`DELETE FROM port_reservations WHERE port = ?`, p); err != nil {
			return err
		}
	}
	return nil
}

func DeleteReservedPortsByOwnerTx(tx *sql.Tx, reservedBy string) error {
	if strings.TrimSpace(reservedBy) == "" {
		return nil
	}
	_, err := tx.Exec(`DELETE FROM port_reservations WHERE reserved_by = ?`, reservedBy)
	return err
}
