# IMUX
`imux` is a Golang library that empowers you to combine multiple instances of `net.Conn` interface into a single, reliable, and ordered `net.Conn`. This is particularly valuable when you encounter limitations on individual sockets and seek to achieve higher throughput. It can also prove beneficial when working to bypass restrictions imposed by systems like the GFW.