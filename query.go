package main

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/miekg/dns"
)

func queryAuthoritative(questionName string, server string) (*dns.Msg, error) {
	client := new(dns.Client)
	client.Timeout = 2 * time.Second // Set the desired timeout duration
	message := new(dns.Msg)
	message.SetQuestion(questionName, dns.TypeA)
	response, _, err := client.Exchange(message, server)
	if err != nil {
		log.Printf("Error querying DNS server (%s) for %s: %s\n", server, questionName, err)
		return nil, err
	}

	if len(response.Answer) == 0 {
		log.Printf("No answer received from DNS server (%s) for %s\n", server, questionName)
		return nil, errors.New("no answer received")
	}

	fmt.Println("response", response.Answer[0].String())

	return response, nil
}

func queryAllDNSServers(question dns.Question, dnsServers []string) <-chan *dns.Msg {
	answers := make(chan *dns.Msg, len(dnsServers))
	var wg sync.WaitGroup

	for _, server := range dnsServers {
		wg.Add(1)
		go func(server string) {
			defer wg.Done()
			authResponse, _ := queryAuthoritative(question.Name, server)
			if authResponse != nil {
				answers <- authResponse
			}
		}(server)
	}

	go func() {
		wg.Wait()
		close(answers)
	}()

	return answers
}

func handleDNSServers(question dns.Question, dnsServers []string, fallbackServer string, response *dns.Msg) {
	answers := queryAllDNSServers(question, dnsServers)

	found := false
	for answer := range answers {
		if answer.MsgHdr.Authoritative {
			processAuthoritativeAnswer(question, answer, response)
			found = true
			break
		}
	}

	if !found {
		handleFallbackServer(question, fallbackServer, response)
	}
}
