package main

import(
  "fmt"
  "crypto/sha1"
  "math/rand"
  "io"
  "time"
  "math"
  "sync/atomic"
  "net"
  "os"
  "runtime"
  "log"
  "github.com/zeebo/bencode"
  "bytes"
  "errors"
  "encoding/hex"
)

//好友节点，带你进入DHT网络
var BOOTSTRAP []string = []string {
  "67.215.246.10:6881", //router.bittorrent.com
  "91.121.59.153:6881", //dht.transmissionbt.com
  "82.221.103.244:6881",//router.utorrent.com
  "212.129.33.50:6881"} 

type Id []byte

type NodeInfo struct {
  Ip       net.IP
  Port     int
  Id       Id
  lastseen time.Time
}

type KNode struct {
  node    *NodeInfo
  routing *Routing
  network *Network
  log     *log.Logger
  krpc    *KRPC
  outChan chan string
  
}

type Network struct {
  dhtNode  *KNode
  Conn     *net.UDPConn
}

type KRPC struct {
  dhtNode  *KNode 
  tid      uint32 
}

//路由表，每个表又分文桶(Bucket)
type Routing struct {
  selfNode   *KNode
  table      []*Bucket
}

//k桶,Bucket
type Bucket struct {
  Nodes      []*NodeInfo
  lastchange time.Time   //新鲜度
}


type KRPCMSG struct {
  T string
  Y string
  Args interface{} 
  addr *net.UDPAddr
}

type Query struct {
  Q string 
  A map[string]interface{}
}

type Response struct {
  R map[string]interface{}
}

//计数
var nums int = 0

func main(){
  cpus := runtime.NumCPU()
  runtime.GOMAXPROCS(cpus)
  master := make(chan string)
  for i := 0; i < cpus; i++ {
    go Executing(cpus, master)
  }
  OutHash(master)
}

func OutHash(master chan string){
  for {
    select {
    case infohash := <-master:
      fmt.Println(infohash)
      nums++
      fmt.Println(nums)
    }
  }
}

func Executing(cpus int, master chan string ){
  dhtNode := NewdhtNode(master, os.Stdout)
  dhtNode.Run()
}

//生成节点id
func GenerateId() Id {
  random := rand.New(rand.NewSource(time.Now().UnixNano()))
  h := sha1.New()
  io.WriteString(h, time.Now().String())
  io.WriteString(h, string(random.Int()))
  return h.Sum(nil)
}

//DhtNode 节点信息
func NewdhtNode(master chan string, logger io.Writer) *KNode{
  dhtNode := new(KNode)
	dhtNode.log = log.New(logger, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)
  dhtNode.node = NewNode()
  dhtNode.routing = NewRouting(dhtNode)
  dhtNode.network = NewNetwork(dhtNode)
  dhtNode.krpc = NewKrpc(dhtNode)
  dhtNode.outChan = master
  return dhtNode
}

func NewNode() *NodeInfo {
  node := new(NodeInfo)
  id := GenerateId()
  node.Id = id
  return node
}

//如果将NewBucket写为一个函数，最后分别赋值table[0]与table[1]那么两个table的节点数量将相同，这是一个坑!
func NewRouting(dhtNode *KNode) *Routing {
  routing := new(Routing)
  routing.selfNode = dhtNode 
  bucket1 := NewBucket()
  bucket2 := NewBucket2()
  routing.table = make([]*Bucket, 2)
  routing.table[0] = bucket1
  routing.table[1] = bucket2
  return routing
}

func NewBucket() *Bucket {
  b := new(Bucket)
  b.Nodes = nil
  b.lastchange = time.Now()
  return b
}

func NewBucket2() *Bucket {
  b := new(Bucket)
  b.Nodes = nil
  b.lastchange = time.Now()
  return b
}

func (bucket *Bucket) Len() int {
  return len(bucket.Nodes)
}

//节点Ip,Port,Id
func NewNetwork(dhtNode *KNode) *Network {
  network := new(Network)
  network.dhtNode = dhtNode
  addr := new(net.UDPAddr)
  var err error
  network.Conn, err = net.ListenUDP("udp", addr)
  if err != nil {
    panic(err)
  }
  laddr := network.Conn.LocalAddr().(*net.UDPAddr)
  network.dhtNode.node.Ip = laddr.IP
  network.dhtNode.node.Port = laddr.Port
  return network
}

//KRPC协议
func NewKrpc(dhtNode *KNode) *KRPC {
  krpc := new(KRPC)
  krpc.dhtNode = dhtNode
  return krpc
}

func (dhtNode *KNode) Run(){
	dhtNode.log.Println(fmt.Sprintf("Current Node ID is %s ", dhtNode.node.Id))
	dhtNode.log.Println(fmt.Sprintf("DhtBT %s is runing...", dhtNode.network.Conn.LocalAddr().String()))

  go func() { dhtNode.network.GetInfohash()}()

  go func() { dhtNode.FindNode()}() 

}

func (dhtNode *KNode) FindNode() {
  for {
    if dhtNode.routing.table[0].Len() == 0{
      dhtNode.searchNodes(dhtNode.node.Id)
    } else {
        for _, node := range dhtNode.routing.table[0].Nodes {
        t := time.Now()
        d, _ := time.ParseDuration("-10s")
        last := t.Add(d)
        ok := node.lastseen.Before(last)
        if ok {
          continue
        }
        dhtNode.GoFindNode( node, GenerateId())
      }
      dhtNode.routing.table[0].Nodes = nil
      time.Sleep(1 * time.Second)
    }
  }
}

func (dhtNode *KNode) searchNodes(target Id) {
  for _, host := range BOOTSTRAP {
    addr, err := net.ResolveUDPAddr("udp", host)
    if err != nil {
      dhtNode.log.Fatalf("Resolve DNS error, %s\n", err)
      return
    }
    node := new(NodeInfo)
    node.Port = addr.Port
    node.Ip = addr.IP
    dhtNode.GoFindNode(node, target)
  }
}

func (table *Bucket) Add(n *NodeInfo) {
  table.Nodes = append(table.Nodes, n)
  table.Updatetime(n)
}

func (bucket *Bucket) Updatetime(n *NodeInfo ) {
  bucket.lastchange = time.Now()
  n.lastseen = time.Now()
}

func (routing *Routing) InsertNode(other *NodeInfo) {
  if routing.isSelf(other) {
    return
  }

  if routing.table[1].Len() < 8 {
    routing.table[1].Add(other)
  }

  routing.table[0].Add(other)
}

func (routing *Routing) isSelf(other *NodeInfo) bool {
  return (routing.selfNode.node.Id.CompareTo(other.Id) == 0)
}

func (id Id) CompareTo(other Id) int {
  s1 := id.HexString()
  s2 := other.HexString()
  if s1 > s2 {
    return 1
  } else if s1 == s2 {
    return 0
  } else {
    return -1
  }
}

func (id *Id) HexString() string {
  return fmt.Sprintf("%x", id)
}

func (dhtNode *KNode) GoFindNode(info *NodeInfo, target Id){
    if info.Ip.Equal(net.IPv4(0, 0, 0, 0)) || info.Port == 0 {
      return
    }
    addr := new(net.UDPAddr)
    addr.IP = info.Ip
    addr.Port = info.Port
    data, err := dhtNode.krpc.EncodingFindNode(target)
    if err != nil {
      dhtNode.log.Println(err)
      return
    }
    err = dhtNode.network.Send([]byte(data), addr)
    if err != nil {
      dhtNode.log.Println(err)
      return
    }
}

func (krpc *KRPC) EncodingFindNode(target Id) (string, error)  {
  tid := krpc.GenTID()
  v := make(map[string]interface{})
  v["t"] = fmt.Sprintf("%d", tid)
  v["y"] = "q"
  v["q"] = "find_node"
  args := make(map[string]string)
  args["id"] = string(krpc.dhtNode.node.Id)
  args["target"] = string(target) //查找自己，找到离自己较近的节点
  v["a"] = args
  s, err := bencode.EncodeString(v)
  if err != nil {
    krpc.dhtNode.log.Fatalln(err)
  }
  return s, err
  
}

func (network *Network) Send(data []byte, addr *net.UDPAddr) error {
    _, err := network.Conn.WriteToUDP(data, addr)
  if err != nil {
    network.dhtNode.log.Println( err)
  } 
  return err
}

func (encode *KRPC) GenTID() uint32 {
  return encode.autoID() % math.MaxUint16
}

func (encode *KRPC) autoID() uint32 {
  return atomic.AddUint32(&encode.tid, 1)
}

//通过网络UDP包获取infohash
//Reads a UDP packet from network.Conn, copying the payload into b. It returns the number of bytes copied into b and the return address that was on the packet
func (network *Network) GetInfohash() {
  b := make([]byte, 1000)
    for {
      _, addr, err := network.Conn.ReadFromUDP(b)
      if err != nil {
        continue
      }
      network.dhtNode.krpc.DecodePackage(string(b), addr)
    }
}

func (krpc *KRPC) DecodePackage(data string, addr *net.UDPAddr) error {
  val := make(map[string]interface{})
  if err := bencode.DecodeString(data, &val); err != nil {
    return err
  } else {
    var ok bool
    msg := new(KRPCMSG) 
    msg.T, ok = val["t"].(string)
    if !ok {
      err = errors.New("Do not have transaction ID ")
      return err
    }
    msg.Y, ok = val["y"].(string)
    if !ok {
      err = errors.New("Do know message type ")
      return err
    }
    msg.addr = addr
    switch msg.Y {
    case "q":
      query := new(Query)
      query.Q = val["q"].(string)
      query.A = val["a"].(map[string]interface{})
      msg.Args = query
      krpc.dhtNode.log.Printf("query.Q is %s", query.Q )
      krpc.Query(msg)
    case "r":
      res := new(Response)
      res.R = val["r"].(map[string]interface{})
      msg.Args = res
      krpc.Response(msg)
    }
    return nil
  }
}

//table[1]不知从哪在自动增加节点数量，远远超过8，待解决
func (krpc *KRPC) Query(msg *KRPCMSG) {
  if query, ok := msg.Args.(*Query); ok {
    queryNode := new(NodeInfo)
    queryNode.Ip = msg.addr.IP
    queryNode.Port = msg.addr.Port 
    queryNode.Id = Id(query.A["id"].(string))
    switch query.Q {
    case "find_node":
      closeNodes := krpc.dhtNode.routing.table[1].Nodes
      nodes := ConvertByteStream(closeNodes)
      data, _ := krpc.EncodingNodeResult(msg.T, "", nodes)
      krpc.dhtNode.network.Send([]byte(data), msg.addr) 
    case "announce_peer":
      if infohash, ok := query.A["info_hash"].(string); ok {
        krpc.dhtNode.outChan <- Id(infohash).String()
      }
    case "get_peers":
      if infohash, ok := query.A["info_hash"].(string); ok {
        krpc.dhtNode.outChan <- Id(infohash).String()
        token := krpc.dhtNode.GenToken(queryNode)
        nodes := ConvertByteStream(krpc.dhtNode.routing.table[1].Nodes)
        data, _ := krpc.EncodingNodeResult(msg.T, token, nodes)
        krpc.dhtNode.network.Send([]byte(data), msg.addr) 
      }
    }
    krpc.dhtNode.routing.InsertNode(queryNode)
  }
}

func (dhtNode *KNode) GenToken(sender *NodeInfo) string {
  h := sha1.New()
  io.WriteString(h, sender.Ip.String())
  io.WriteString(h, time.Now().String())
  token := bytes.NewBuffer(h.Sum(nil)).String()
  return token
}
 
func ConvertByteStream(nodes []*NodeInfo) []byte {
  buf := bytes.NewBuffer(nil)
  for _, v := range nodes {
    convertNodeInfo(buf, v)
  }
  return buf.Bytes()
}

func convertNodeInfo(buf *bytes.Buffer, v *NodeInfo) {
  buf.Write(v.Id)
  convertIPPort(buf, v.Ip, v.Port)
}

func convertIPPort(buf *bytes.Buffer, ip net.IP, port int) {
  buf.Write(ip.To4())
  buf.WriteByte(byte((port & 0xFF00) >> 8))
  buf.WriteByte(byte(port & 0xFF))
}

func (krpc *KRPC) EncodingNodeResult(tid string, token string, nodes []byte) (string, error) {
  v := make(map[string]interface{})
  v["t"] = tid
  v["y"] = "r"
  args := make(map[string]string)
  args["id"] = krpc.dhtNode.node.Id.String()
  if token != "" {
    args["token"] = token
  }
  args["nodes"] = bytes.NewBuffer(nodes).String()
  v["r"] = args
  s, err := bencode.EncodeString(v)
  return s, err
}

func (id Id) String() string {
  return hex.EncodeToString(id)
}

func (krpc *KRPC) Response(msg *KRPCMSG) {
  if res, ok := msg.Args.(*Response); ok {
    if nodestr, ok := res.R["nodes"].(string); ok {
      nodes := ParseBytesStream([]byte(nodestr))
      for _, v := range nodes {
        krpc.dhtNode.routing.InsertNode(v)
      }
    }
  }
}

func ParseBytesStream(data []byte) []*NodeInfo {
  var nodes []*NodeInfo = nil
  for j := 0; j < len(data); j = j + 26 {
    if j+26 > len(data) {
      break
    }
    kn := data[j : j+26]
    node := new(NodeInfo)
    node.Id = Id(kn[0:20])
    node.Ip = kn[20:24]
    port := kn[24:26]
    node.Port = int(port[0])<<8 + int(port[1])
    nodes = append(nodes, node)
  }
  return nodes
}
