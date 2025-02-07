package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nats-io/nats.go"
)

func main() {
	// Conectar ao servidor NATS
	nc, err := nats.Connect("nats://manager.evolution-api.io:4222")
	if err != nil {
		log.Fatalf("Erro ao conectar ao NATS: %v", err)
	}
	defer nc.Close()

	// Criar canal para receber sinais de interrupção
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Lista de eventos conforme são publicados (em lowercase)
	eventos := []string{
		"message",
		"receipt",
		"presence",
		"historysync",
		"chatpresence",
		"calloffer",
		"callaccept",
		"callterminate",
		"calloffernotice",
		"callrelaylatency",
		"connected",
		"pairsuccess",
		"temporaryban",
		"loggedout",
		"connectfailure",
		"disconnected",
		"labeledit",
		"labelassociationchat",
		"labelassociationmessage",
		"contact",
		"pushname",
		"groupinfo",
		"joinedgroup",
		"newsletterjoin",
		"newsletterleave",
		"qrcode",
		"qrtimeout",
		"qrsuccess",
	}

	// Subscrever a todos os eventos
	for _, evento := range eventos {
		// Garantir que o nome do evento está em lowercase
		eventoLower := strings.ToLower(evento)
		_, err := nc.QueueSubscribe(eventoLower, "whatsapp_events_queue", func(msg *nats.Msg) {
			fmt.Printf("Recebido [%s]: %s\n", msg.Subject, string(msg.Data))
		})
		if err != nil {
			log.Printf("Erro ao subscrever ao evento %s: %v", eventoLower, err)
			continue
		}
		fmt.Printf("Subscrito ao evento: %s na fila: whatsapp_events_queue\n", eventoLower)
	}

	fmt.Println("Consumidor NATS iniciado. Pressione Ctrl+C para sair.")

	// Aguardar sinal de interrupção
	<-sigChan
	fmt.Println("\nEncerrando consumidor...")
}
