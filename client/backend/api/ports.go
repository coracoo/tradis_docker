package api

import (
	"bufio"
	"context"
	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/docker"
	"dockerpanel/backend/pkg/settings"
	"errors"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/gin-gonic/gin"
)

func RegisterPortRoutes(r *gin.RouterGroup) {
	group := r.Group("/ports")
	{
		group.GET("", listPorts)
		group.GET("/list", listPorts)
		group.GET("/range", getPortRange)
		group.POST("/range", updatePortRange)
		group.POST("/note", savePortNote)
		group.POST("/allocate", allocatePorts)
		group.POST("/release", releasePorts)
	}
}

type PortRecord struct {
	Port     int    `json:"port"`
	EndPort  int    `json:"end_port"`
	Type     string `json:"type"`
	Protocol string `json:"protocol"`
	Used     bool   `json:"used"`
	Note     string `json:"note"`
	Service  string `json:"service"`
}

type PortUsage struct {
	Used        bool
	Type        string
	ServiceName string
}

func autoPortAllocationAllowed(reservedBy string, enabled bool) bool {
	return enabled
}

func listPorts(c *gin.Context) {
	start := parseIntDefault(c.Query("start"), 0)
	end := parseIntDefault(c.Query("end"), 65535)
	if start < 0 {
		start = 0
	}
	if end > 65535 {
		end = 65535
	}
	if end < start {
		end = start
	}

	protocolFilter := strings.ToLower(strings.TrimSpace(c.Query("protocol")))
	if protocolFilter == "" {
		protocolFilter = "all"
	}

	typeFilterRaw := strings.ToLower(strings.TrimSpace(c.Query("type")))
	typeFilter := strings.Title(typeFilterRaw)
	if typeFilterRaw == "" {
		typeFilterRaw = "all"
		typeFilter = "All"
	}
	typeValues := map[string]bool{}
	if typeFilterRaw != "all" {
		for _, part := range strings.Split(typeFilterRaw, ",") {
			v := strings.ToLower(strings.TrimSpace(part))
			if v != "" && v != "all" {
				typeValues[v] = true
			}
		}
	}
	if len(typeValues) == 0 {
		typeValues = nil
	}
	page := parseIntDefault(c.Query("page"), 1)
	pageSize := parseIntDefault(c.Query("pageSize"), 50)
	if pageSize <= 0 {
		pageSize = 50
	}
	searchPort := c.Query("search")
	exactPort := -1
	if p, err := strconv.Atoi(strings.TrimSpace(searchPort)); err == nil {
		exactPort = p
	}

	usedFilter := strings.ToLower(strings.TrimSpace(c.Query("used")))
	if usedFilter == "" {
		usedFilter = "all"
	}

	tcpUsage, udpUsage, err := gatherDetailedPortUsage()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取端口占用失败", err)
		return
	}

	notes, err := database.GetAllPortNotes()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取端口备注失败", err)
		return
	}
	reserved, err := database.GetReservedPorts()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取端口预留失败", err)
		return
	}
	for p := range reserved {
		if p < start || p > end {
			continue
		}
		if !tcpUsage[p].Used {
			tcpUsage[p] = PortUsage{Used: true, Type: "Reserved", ServiceName: "reserved"}
		}
		if !udpUsage[p].Used {
			udpUsage[p] = PortUsage{Used: true, Type: "Reserved", ServiceName: "reserved"}
		}
	}

	opts := FilterOptions{
		Start:      start,
		End:        end,
		Protocol:   protocolFilter,
		TypeValues: typeValues,
		Used:       usedFilter,
		ExactPort:  exactPort,
	}

	var rawItems []PortRecord

	// TCP
	if protocolFilter == "all" || protocolFilter == "tcp" {
		tcpRecords := processPorts(start, end, "TCP", tcpUsage, opts, notes)
		rawItems = append(rawItems, tcpRecords...)
	}

	// UDP
	if protocolFilter == "all" || protocolFilter == "udp" {
		udpRecords := processPorts(start, end, "UDP", udpUsage, opts, notes)
		rawItems = append(rawItems, udpRecords...)
	}

	// Calculate stats on aggregated items
	usedCount := 0
	availableCount := 0
	for _, it := range rawItems {
		count := it.EndPort - it.Port + 1
		if it.Used {
			usedCount += count
		} else {
			availableCount += count
		}
	}

	total := len(rawItems)
	startIndex := (page - 1) * pageSize
	if startIndex < 0 {
		startIndex = 0
	}
	if startIndex > total {
		startIndex = total
	}
	endIndex := startIndex + pageSize
	if endIndex > total {
		endIndex = total
	}

	c.JSON(200, gin.H{
		"items":           rawItems[startIndex:endIndex],
		"total":           total,
		"used":            usedCount,
		"available":       availableCount,
		"range_start":     start,
		"range_end":       end,
		"protocol_filter": protocolFilter,
		"type_filter":     typeFilter,
		"page":            page,
		"pageSize":        pageSize,
	})
}

func savePortNote(c *gin.Context) {
	var req struct {
		Port     int    `json:"port"`
		Type     string `json:"type"`
		Protocol string `json:"protocol"`
		Note     string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "无效的请求参数", err)
		return
	}
	if req.Port < 0 || req.Port > 65535 {
		respondError(c, http.StatusBadRequest, "端口必须在 0-65535 范围内", nil)
		return
	}

	tx, err := database.GetDB().Begin()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "创建事务失败", err)
		return
	}
	if err := database.SavePortNoteTx(tx, req.Port, req.Type, strings.ToUpper(req.Protocol), req.Note); err != nil {
		_ = tx.Rollback()
		respondError(c, http.StatusInternalServerError, "保存端口备注失败", err)
		return
	}
	if err := tx.Commit(); err != nil {
		respondError(c, http.StatusInternalServerError, "提交事务失败", err)
		return
	}
	c.JSON(200, gin.H{"message": "备注已保存"})
}

func getPortRange(c *gin.Context) {
	s, err := database.GetPortRange()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取端口范围失败", err)
		return
	}
	c.JSON(200, s)
}

func updatePortRange(c *gin.Context) {
	var req struct {
		Start    int    `json:"start"`
		End      int    `json:"end"`
		Protocol string `json:"protocol"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "无效的请求参数", err)
		return
	}
	if req.Start < 0 {
		req.Start = 0
	}
	if req.End > 65535 {
		req.End = 65535
	}
	if req.End < req.Start {
		req.End = req.Start
	}
	proto := strings.ToLower(strings.TrimSpace(req.Protocol))
	if proto == "" {
		proto = "tcp+udp"
	}

	tx, err := database.GetDB().Begin()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "创建事务失败", err)
		return
	}
	if err := database.SavePortRangeTx(tx, req.Start, req.End, strings.ToUpper(proto)); err != nil {
		_ = tx.Rollback()
		respondError(c, http.StatusInternalServerError, "保存端口范围失败", err)
		return
	}
	if err := tx.Commit(); err != nil {
		respondError(c, http.StatusInternalServerError, "提交事务失败", err)
		return
	}
	c.JSON(200, gin.H{"message": "范围已更新"})
}

func allocatePorts(c *gin.Context) {
	var req struct {
		Count         int    `json:"count"`
		Protocol      string `json:"protocol"`
		Type          string `json:"type"`
		Counts        []int  `json:"counts"`
		ReservedBy    string `json:"reservedBy"`
		UseAllocRange bool   `json:"useAllocRange"`
		DryRun        bool   `json:"dryRun"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "无效的请求参数", err)
		return
	}
	if req.Count < 0 {
		respondError(c, http.StatusBadRequest, "分配数量不能小于 0", nil)
		return
	}
	for _, cnt := range req.Counts {
		if cnt <= 0 {
			respondError(c, http.StatusBadRequest, "连续端口数量必须大于 0", nil)
			return
		}
	}
	if req.Count == 0 && len(req.Counts) == 0 {
		req.Count = 1
	}

	var startP, endP int
	reservedBy := strings.TrimSpace(req.ReservedBy)
	if reservedBy == "" {
		reservedBy = "deploy"
	}

	if req.UseAllocRange {
		s, err := settings.GetSettings()
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Failed to load allocation settings", err)
			return
		}
		if !autoPortAllocationAllowed(reservedBy, s.AllowAutoAllocPort) {
			respondError(c, http.StatusForbidden, "自动端口分配未启用，请先在设置中开启", nil)
			return
		}
		startP = s.AllocPortStart
		endP = s.AllocPortEnd
		// Debug log
		log.Printf("Allocating from settings range: %d-%d (UseAllocRange=true)", startP, endP)

		// Validate
		if startP <= 0 {
			startP = 55500
		}
		if endP <= 0 {
			endP = 56000
		}
		if endP < startP {
			endP = startP
		}
	} else {
		rng, err := database.GetPortRange()
		if err != nil {
			respondError(c, http.StatusInternalServerError, "获取端口范围失败", err)
			return
		}
		startP = rng.Start
		endP = rng.End
	}

	proto := strings.ToLower(strings.TrimSpace(req.Protocol))
	if proto == "" || proto == "all" || proto == "tcp+udp" {
		proto = "tcp"
	}
	t := strings.Title(strings.ToLower(strings.TrimSpace(req.Type)))
	if t == "" {
		t = "Host"
	}

	tcpUsage, udpUsage, err := gatherDetailedPortUsage()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取端口占用失败", err)
		return
	}

	reserved, _ := database.GetReservedPorts()
	used := map[int]bool{}

	if startP < 1 {
		startP = 1
	}

	usedCount := 0
	for p := startP; p <= endP; p++ {
		if proto == "tcp" {
			if tcpUsage[p].Used {
				used[p] = true
			}
		} else {
			if udpUsage[p].Used {
				used[p] = true
			}
		}
		if reserved[p] {
			used[p] = true
		}
		if used[p] {
			usedCount++
		}
	}

	log.Printf("Range %d-%d check: Used %d/%d ports", startP, endP, usedCount, endP-startP+1)

	var segments [][]int
	if len(req.Counts) > 0 {
		for _, cnt := range req.Counts {
			seg, ferr := FindContiguousPorts(used, startP, endP, cnt)
			if ferr != nil {
				log.Printf("Failed to find %d ports in range %d-%d: %v. First few used: %v", cnt, startP, endP, ferr, getFirstFewUsed(used, startP, 10))
				respondError(c, http.StatusConflict, ferr.Error(), nil)
				return
			}
			segments = append(segments, seg)
			for _, p := range seg {
				used[p] = true
			}
		}
	} else {
		seg, ferr := FindContiguousPorts(used, startP, endP, req.Count)
		if ferr != nil {
			log.Printf("Failed to find %d ports in range %d-%d: %v. First few used: %v", req.Count, startP, endP, ferr, getFirstFewUsed(used, startP, 10))
			respondError(c, http.StatusConflict, ferr.Error(), nil)
			return
		}
		segments = [][]int{seg}
	}

	if req.DryRun {
		c.JSON(200, gin.H{"segments": segments})
		return
	}

	tx, err := database.GetDB().Begin()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "创建事务失败", err)
		return
	}
	for _, seg := range segments {
		if err := database.ReservePortsTx(tx, seg, reservedBy, strings.ToUpper(proto), t); err != nil {
			_ = tx.Rollback()
			respondError(c, http.StatusInternalServerError, "预留端口失败", err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		respondError(c, http.StatusInternalServerError, "提交事务失败", err)
		return
	}

	c.JSON(200, gin.H{"segments": segments})
}

func releasePorts(c *gin.Context) {
	var req struct {
		Ports      []int  `json:"ports"`
		ReservedBy string `json:"reservedBy"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "无效的请求参数", err)
		return
	}
	if len(req.Ports) == 0 && strings.TrimSpace(req.ReservedBy) == "" {
		respondError(c, http.StatusBadRequest, "必须提供 ports 或 reservedBy", nil)
		return
	}

	tx, err := database.GetDB().Begin()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "创建事务失败", err)
		return
	}
	if len(req.Ports) > 0 {
		if err := database.DeleteReservedPortsByPortsTx(tx, req.Ports); err != nil {
			_ = tx.Rollback()
			respondError(c, http.StatusInternalServerError, "释放端口失败", err)
			return
		}
	}
	if strings.TrimSpace(req.ReservedBy) != "" {
		if err := database.DeleteReservedPortsByOwnerTx(tx, req.ReservedBy); err != nil {
			_ = tx.Rollback()
			respondError(c, http.StatusInternalServerError, "释放端口失败", err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		respondError(c, http.StatusInternalServerError, "提交事务失败", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "端口预留已释放"})
}

func FindContiguousPorts(used map[int]bool, start, end, count int) ([]int, error) {
	if count <= 0 {
		return []int{}, nil
	}
	run := 0
	begin := start
	for p := start; p <= end; p++ {
		if used[p] {
			run = 0
			begin = p + 1
			continue
		}
		if run == 0 {
			begin = p
		}
		run++
		if run >= count {
			seg := make([]int, count)
			for i := 0; i < count; i++ {
				seg[i] = begin + i
			}
			return seg, nil
		}
	}
	return nil, errors.New("没有足够连续的可用端口")
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return i
}

func gatherDetailedPortUsage() (map[int]PortUsage, map[int]PortUsage, error) {
	tcpUsage := make(map[int]PortUsage)
	udpUsage := make(map[int]PortUsage)

	for i := 0; i <= 65535; i++ {
		tcpUsage[i] = PortUsage{Used: false}
		udpUsage[i] = PortUsage{Used: false}
	}

	sysTCP, sysUDP, err := getSystemPorts()
	if err != nil {
		return nil, nil, err
	}

	cli, err := docker.NewDockerClient()
	if err != nil {
		return nil, nil, err
	}
	defer cli.Close()
	containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{All: true})
	if err != nil {
		return nil, nil, err
	}

	containerMapTCP := make(map[int]string)
	containerMapUDP := make(map[int]string)

	for _, c := range containers {
		for _, p := range c.Ports {
			if p.PublicPort == 0 {
				continue
			}
			name := strings.TrimPrefix(c.Names[0], "/")
			if strings.ToLower(p.Type) == "tcp" {
				containerMapTCP[int(p.PublicPort)] = name
			} else if strings.ToLower(p.Type) == "udp" {
				containerMapUDP[int(p.PublicPort)] = name
			}
		}
	}

	for p, info := range sysTCP {
		usage := PortUsage{Used: true, Type: "Host", ServiceName: info.ProcessName}

		// If Docker knows about this port, it's a container port, regardless of process name
		if name, ok := containerMapTCP[p]; ok {
			usage.Type = "Container"
			usage.ServiceName = name
		} else {
			// Fallback: check process name
			isDockerProxy := strings.Contains(info.ProcessName, "docker-proxy")
			if isDockerProxy {
				usage.Type = "Container"
				usage.ServiceName = "docker-proxy"
			}
		}
		tcpUsage[p] = usage
	}

	for p, info := range sysUDP {
		usage := PortUsage{Used: true, Type: "Host", ServiceName: info.ProcessName}

		if name, ok := containerMapUDP[p]; ok {
			usage.Type = "Container"
			usage.ServiceName = name
		} else {
			isDockerProxy := strings.Contains(info.ProcessName, "docker-proxy")
			if isDockerProxy {
				usage.Type = "Container"
				usage.ServiceName = "docker-proxy"
			}
		}
		udpUsage[p] = usage
	}

	return tcpUsage, udpUsage, nil
}

type SysPortInfo struct {
	PID         string
	ProcessName string
}

func getSystemPorts() (map[int]SysPortInfo, map[int]SysPortInfo, error) {
	tcpPorts := make(map[int]SysPortInfo)
	udpPorts := make(map[int]SysPortInfo)

	cmd := exec.Command("netstat", "-tulnp")
	out, err := cmd.StdoutPipe()
	if err != nil {
		// Try without p if it fails? No, user has sudo or enough perms usually.
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	defer func() { _ = cmd.Wait() }()

	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}

		proto := strings.ToLower(fields[0])
		localAddr := fields[3]

		// Find PID/Program column. It's usually the last one, or second to last?
		// netstat -tulnp output:
		// Proto Recv-Q Send-Q Local Address           Foreign Address         State       PID/Program name
		// tcp        0      0 0.0.0.0:22              0.0.0.0:*               LISTEN      1234/sshd
		// udp ...                                                                         1234/sshd
		// Note: State column exists for TCP but NOT for UDP usually?
		// Actually netstat -tulnp:
		// Active Internet connections (only servers)
		// Proto Recv-Q Send-Q Local Address           Foreign Address         State       PID/Program name
		// tcp ... LISTEN ...
		// udp ...        ...

		// Let's parse from right to left or by column count.
		// If TCP, 7 columns? Proto, Recv, Send, Local, Foreign, State, PID/Prog
		// If UDP, 6 columns? Proto, Recv, Send, Local, Foreign, PID/Prog

		var pidProg string
		if strings.HasPrefix(proto, "tcp") {
			if len(fields) >= 7 {
				pidProg = fields[6]
			}
		} else if strings.HasPrefix(proto, "udp") {
			if len(fields) >= 6 {
				pidProg = fields[5]
			}
		}

		if pidProg == "" || pidProg == "-" {
			// Try last field if it looks like PID/Prog
			last := fields[len(fields)-1]
			if strings.Contains(last, "/") {
				pidProg = last
			}
		}

		lastColon := strings.LastIndex(localAddr, ":")
		if lastColon == -1 {
			continue
		}
		portStr := localAddr[lastColon+1:]
		port, err := strconv.Atoi(portStr)
		if err != nil {
			continue
		}

		var pid, prog string
		if slashIdx := strings.Index(pidProg, "/"); slashIdx != -1 {
			pid = pidProg[:slashIdx]
			prog = pidProg[slashIdx+1:]
		} else {
			prog = pidProg
		}
		if prog == "" {
			prog = "Unknown"
		}

		info := SysPortInfo{PID: pid, ProcessName: prog}

		if strings.Contains(proto, "tcp") {
			tcpPorts[port] = info
		} else if strings.Contains(proto, "udp") {
			udpPorts[port] = info
		}
	}

	// Fallback: Scan local listeners if netstat returned nothing?
	// But netstat -tulnp is standard.
	// If tcpPorts is empty, maybe netstat failed silently or output format diff?
	// User provided output shows standard format.

	return tcpPorts, udpPorts, nil
}

func getFirstFewUsed(used map[int]bool, start int, limit int) []int {
	var res []int
	count := 0
	// just check first 100 ports from start to see if they are used
	for p := start; p < start+100; p++ {
		if used[p] {
			res = append(res, p)
			count++
			if count >= limit {
				break
			}
		}
	}
	return res
}
