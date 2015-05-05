# DhtBT
Golang实现的基于DHT分布式存储网络爬虫, 抱着学习 Golang 的目的。

## DHT 网络 
DHT 全称叫分布式哈希表(Distributed Hash Table)，是一种分布式存储方法。在不需要服务器的情况下，每个客户端负责一个小范围的路由，并负责存储一小部分数据，从而实现整个DHT网络的寻址和存储。简单的说，DHT 网络中的所有资源都是分散存储在网络中的节点中的，并不是集中存储在某一台服务器上。这样的好处就是防止因为服务器问题或是线路问题而影响所有资源的下载，同时也极大的减小了一台服务器的负载。

## KRPC 协议
KRPC 协议是由 bencode 编码组成的一个简单的 RPC 结构，他使用 UDP 报文发送。支撑了DHT网络里信息分布式存储的实现。

一个独立的请求包被发出去然后一个独立的包被回复。这个协议没有重发。它包含 3 种消息：请求，回复和错误。对DHT协议而言，这里有 4 种请求：`ping` ，`find_node`，`get_peers` 和 `announce_peer`。

## 依赖

```
 $ go get github.com/zeebo/bencode
```

## 参考

* [DHT Protocol](http://www.bittorrent.org/beps/bep_0005.html)
* [P2P中DHT网络爬虫](http://codemacro.com/2013/05/19/crawl-dht/)
* [如何“养”一只DHT爬虫](http://developer.51cto.com/art/201402/430007_all.htm)
