package bngsocket

import (
	"bufio"
	"encoding/binary"
	"fmt"
)

// Ließt Chunks ein und verarbeitet sie weiter
func constantReading(o *BngConn) {
	defer func() {
		// DEBUG Log
		DebugPrint(fmt.Sprintf("BngConn(%s): Constant reading from Socket was stopped", o._innerhid))

		// Es wird eine Hintergrundaufgabe für erledigt Markeirt
		o.backgroundProcesses.Done()
	}()

	// Der Buffer Reader ließt die Daten und speichert sie dann im Cache
	reader := bufio.NewReader(o.conn)
	cacheBytes := make([]byte, 0)
	cachedData := make([]byte, 0)

	// Debug
	DebugPrint(fmt.Sprintf("BngConn(%s): Constant reading from Socket was started", o._innerhid))

	// Diese Schleife wird Permanent ausgeführt und liest alle Daten ein
	for runningBackgroundServingLoop(o) {
		// Verfügbare Daten aus der Verbindung lesen
		rBytes := make([]byte, 4096)
		sizeN, err := reader.Read(rBytes)
		if err != nil {
			readProcessErrorHandling(o, err)
			return
		}

		// Die Daten werden im Cache zwischengespeichert
		cacheBytes = append(cacheBytes, rBytes[:sizeN]...)

		// Verarbeitung des Empfangspuffers
		ics := 0
		for {
			// Es wird geprüft ob Mindestens 1 Byte im Cache ist
			if len(cacheBytes) < 1 {
				break // Nicht genug Daten, um den Nachrichtentyp zu bestimmen
			}

			// Der Datentyp wird ermittelt
			if cacheBytes[0] == byte('C') {
				ics = ics + 1

				// Prüfen, ob genügend Daten für die Chunk-Länge vorhanden sind
				if len(cacheBytes) < 3 { // 1 Byte für 'C' und 2 Bytes für die Länge
					break // Warten auf mehr Daten
				}

				// Chunk-Länge lesen (Bytes 1 bis 2)
				length := binary.BigEndian.Uint16(cacheBytes[1:3])

				// Optional: Maximale erlaubte Chunk-Größe überprüfen
				const MaxChunkSize = 4096 // Maximal erlaubte Chunk-Größe in Bytes
				if length > MaxChunkSize {
					// Der Fehler wird ausgewertet
					readProcessErrorHandling(o, fmt.Errorf("chunk too large: %d bytes", length))
					break
				}

				// Prüfen, ob genügend Daten für den gesamten Chunk vorhanden sind
				totalLength := 1 + 2 + int(length) // 'C' + Länge (2 Bytes) + Chunk-Daten
				if len(cacheBytes) < totalLength {
					break // Warten auf mehr Daten
				}

				// Chunk-Daten extrahieren
				chunkData := cacheBytes[3:totalLength]

				// Chunk-Daten dem gecachten Daten hinzufügen
				cachedData = append(cachedData, chunkData...)

				// Verarbeitete Bytes aus dem Cache entfernen
				cacheBytes = cacheBytes[totalLength:]
			} else if cacheBytes[0] == byte('L') {
				// 'L' aus dem Cache entfernen
				cacheBytes = cacheBytes[1:]
				ics = ics + 1

				// Gesammelte Daten verarbeiten
				transportBytes := make([]byte, len(cachedData))
				copy(transportBytes, cachedData)
				cachedData = make([]byte, 0)
				o.backgroundProcesses.Add(1)

				// Debug
				DebugPrint(fmt.Sprintf("BngConn(%s): %d bytes was recived", o._innerhid, len(transportBytes)+ics))

				// Die Daten werden durch die GoRoutine verarbeitet
				go func(data []byte) {
					defer o.backgroundProcesses.Done()
					o._ProcessReadedData(data)
				}(transportBytes)
			} else {
				// Der Fehler wird ausgewertet
				readProcessErrorHandling(o, fmt.Errorf("unknown message type: %v", cacheBytes[0]))
				break
			}
		}
	}
}