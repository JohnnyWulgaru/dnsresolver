package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/miekg/dns"
)

func getCacheRecords() ([]CacheRecord, error) {
	var cacheRecords []CacheRecord
	var records Records

	data, err := os.ReadFile("cache.json")
	if err != nil {
		return cacheRecords, err
	}

	err = json.Unmarshal(data, &records)
	if err != nil {
		return cacheRecords, err
	}

	cacheRecords = make([]CacheRecord, len(records.Records))
	for i, record := range records.Records {
		cacheRecords[i].DNSRecord = record
		cacheRecords[i].Timestamp = time.Now()
		cacheRecords[i].Expiry = record.LastQuery.Add(time.Duration(record.TTL) * time.Second)
	}
	return cacheRecords, nil
}

func saveCacheRecords(cacheRecords []CacheRecord) {
	records := Records{Records: make([]DNSRecord, len(cacheRecords))}
	for i, cacheRecord := range cacheRecords {
		records.Records[i] = cacheRecord.DNSRecord
		records.Records[i].TTL = uint32(cacheRecord.Expiry.Sub(cacheRecord.Timestamp).Seconds())
		records.Records[i].LastQuery = cacheRecord.LastQuery
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		log.Println("Error marshalling cache records:", err)
		return
	}
	err = os.WriteFile("cache.json", data, 0644)
	if err != nil {
		log.Println("Error saving cache records:", err)
	}
}

func addToCache(cacheRecords []CacheRecord, record *dns.RR) []CacheRecord {

	if !dnsServerSettings.CacheRecords {
		return cacheRecords
	}

	var value string
	switch r := (*record).(type) {
	case *dns.A:
		value = r.A.String()
	case *dns.AAAA:
		value = r.AAAA.String()
	case *dns.CNAME:
		value = r.Target
	case *dns.MX:
		value = fmt.Sprintf("%d %s", r.Preference, r.Mx)
	case *dns.NS:
		value = r.Ns
	case *dns.SOA:
		value = fmt.Sprintf("%s %s %d %d %d %d %d", r.Ns, r.Mbox, r.Serial, r.Refresh, r.Retry, r.Expire, r.Minttl)
	case *dns.TXT:
		value = strings.Join(r.Txt, " ")
	default:
		value = (*record).String()
	}

	cacheRecord := CacheRecord{
		DNSRecord: DNSRecord{
			Name:  (*record).Header().Name,
			Type:  dns.TypeToString[(*record).Header().Rrtype],
			Value: value,
			TTL:   (*record).Header().Ttl,
		},
		Expiry:    time.Now().Add(time.Duration((*record).Header().Ttl) * time.Second),
		Timestamp: time.Now(), // Add this line
	}

	// Check if the record already exists in the cache
	recordIndex := -1
	for i, existingRecord := range cacheRecords {
		if existingRecord.DNSRecord.Name == cacheRecord.DNSRecord.Name &&
			existingRecord.DNSRecord.Type == cacheRecord.DNSRecord.Type &&
			existingRecord.DNSRecord.Value == cacheRecord.DNSRecord.Value {
			recordIndex = i
			break
		}
	}

	// If the record exists in the cache, update its TTL, expiry, and last query, otherwise add it
	if recordIndex != -1 {
		cacheRecords[recordIndex].DNSRecord.TTL = cacheRecord.DNSRecord.TTL
		cacheRecords[recordIndex].Expiry = cacheRecord.Expiry
		cacheRecords[recordIndex].LastQuery = time.Now()
	} else {
		cacheRecord.LastQuery = time.Now()
		cacheRecords = append(cacheRecords, cacheRecord)
	}

	saveCacheRecords(cacheRecords)
	return cacheRecords
}
