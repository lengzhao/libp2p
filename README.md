# libp2p

1. 用于分布式点对点传输
2. 支持多种传输协议
3. url格式的地址，如：tcp://0c61f4028177@127.0.0.1:54300
4. 不同节点可以使用不同的协议
5. 同时方便扩展新的协议
6. 支持插件式功能扩展
7. 当前支持的协议有：tcp,udp,ws,s2s
8. tcp:最基本用法，可靠连接
9. udp：允许丢包，避免阻塞
10. ws: 基于http的Websocket连接，可以穿透防火墙，方便部署到http server的后端
11. s2s：udp server to udp server，只会占用一个端口，连接数量无限，方便NAT穿透
12. 使用方式可以参考examples/chat
