# libp2p

1. 支持多种传输协议
2. 不同节点可以使用不同的协议
3. 同时方便扩展新的协议
4. 当前支持的协议有：TCP,UDP,Websocket,s2s
5. tcp:最基本用法，可靠连接
6. UDP：允许丢包，避免阻塞
7. Websocket：基于http的ws连接，可以穿透防火墙
8. s2s：udp server to udp server，只会占用一个端口，连接数量无限，方便NAT穿透
