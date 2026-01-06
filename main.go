package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
)

var allowedSites = []string{
	"yandex.ru",
	"ya.ru",
	"vk.com",
	"mail.ru",
	"ok.ru",
	"max.ru",
	"gosuslugi.ru",
	"tinkoff.ru",
	"sber.ru",
	"alfa-bank.ru",
	"gazprombank.ru",
	"vtb.ru",
	"ozon.ru",
	"wildberries.ru",
	"avito.ru",
	"drom.ru",
	"cian.ru",
	"pochta.ru",
	"mos.ru",
	"nalog.gov.ru",
	"kremlin.ru",
	"ria.ru",
	"yandex.net",
}

var normalSites = []string{
	"google.com",
	"wikipedia.org",
	"youtube.com",
	"github.com",
	"reddit.com",
	"steamcommunity.com",
	"t.me",
	"chatgpt.com",
	"archive.org",
	"cloudflare.com",
	"openstreetmap.org",
	"mozilla.org",
	"example.org",
	"ovh.com",
	"spotify.com",
}

var workers = 10

var httpClient = &http.Client{
	Timeout: 6 * time.Second,
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	},
}

type Result struct {
	Host   string
	DNS    bool
	TCP80  bool
	TCP443 bool
	HTTP   bool
}

func check(host string) Result {
	r := Result{Host: host}

	// dns
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err == nil && len(ips) > 0 {
		r.DNS = true
	}

	// tcp
	dialer := &net.Dialer{Timeout: 4 * time.Second}

	if r.DNS {
		if c, err := dialer.Dial("tcp", host+":80"); err == nil {
			r.TCP80 = true
			c.Close()
		}
		if c, err := dialer.Dial("tcp", host+":443"); err == nil {
			r.TCP443 = true
			c.Close()
		}
	}

	// http(s)
	for _, u := range []string{
		"https://" + host,
		"http://" + host,
	} {
		resp, err := httpClient.Get(u)
		if err == nil {
			r.HTTP = true
			resp.Body.Close()
			break
		}
	}

	return r
}

func run(sites []string) []Result {
	out := make([]Result, len(sites))

	var wg sync.WaitGroup
	wg.Add(len(sites))

	sem := make(chan struct{}, workers)

	bar := progressbar.NewOptions(len(sites),
		progressbar.OptionSetDescription("Проверка сайтов"),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(30),
		progressbar.OptionClearOnFinish(),
	)

	progressChan := make(chan struct{}, len(sites))

	for i, s := range sites {
		sem <- struct{}{}

		go func(i int, host string) {
			defer wg.Done()
			defer func() { <-sem }()

			out[i] = check(host)
			progressChan <- struct{}{}
		}(i, s)
	}

	go func() {
		for range progressChan {
			bar.Add(1)
		}
	}()

	wg.Wait()
	close(progressChan)

	return out
}

func status(r Result) string {
	switch {
	case r.HTTP:
		return "OK"
	case (r.TCP80 || r.TCP443):
		return "TCP"
	case r.DNS:
		return "DNS"
	default:
		return "FAIL"
	}
}

func printResults(title string, res []Result) {
	fmt.Println("\n", title)
	fmt.Println("------------------------")
	for _, r := range res {
		fmt.Printf("%-20s  %s\n", r.Host, status(r))
	}
}

func score(results []Result) float64 {
	if len(results) == 0 {
		return 0
	}
	ok := 0
	for _, r := range results {
		if r.HTTP {
			ok++
		}
	}
	return float64(ok) / float64(len(results))
}

func main() {
	fmt.Println("WLC запущен...")

	allowed := run(allowedSites)
	normal := run(normalSites)

	printResults("Сайты белого списка", allowed)
	printResults("Обычные сайты", normal)

	rAllowed := score(allowed)
	rNormal := score(normal)

	fmt.Println("\nИтог:")
	fmt.Printf("Разрешённые сайты доступны: %.0f%%\n", rAllowed*100)
	fmt.Printf("Обычные сайты доступны:     %.0f%%\n", rNormal*100)

	switch {
	case rAllowed > 0.6 && rNormal < 0.3:
		fmt.Println("Вероятно активен белый список")
	case rAllowed > 0.6 && rNormal > 0.6:
		fmt.Println("Интернет работает нормально")
	case rAllowed < 0.3 && rNormal < 0.3:
		fmt.Println("Интернет почти полностью недоступен")
	default:
		fmt.Println("Результат неоднозначен.")
	}
}
