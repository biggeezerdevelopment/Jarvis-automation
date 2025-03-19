# Jarvis-automation
 Jarvis-automation is an automation software that uses an agent and mqtt for control like opendxl.

 It will be able to send commands to a set of servers or specified server or all servers to run tasks.  And 
 gather that information back for processing.

# Goals

1.  All traffic is encrypted to and from the servers and the mqtt/syslog/api endpoints.
2.  Web management front end that will display/graph the information sent and cmdcontrol
3.  cmdcontrol application from the cli 

# Just some Tasks the agent will do

1. Gather telementry of the server (cpu/network/disk/memory)
2. Gather process information
3. Gather Startup information
4. Gather registry information
5. Run yara rules
6. Run remote api gathering processes
7. run yara rules on in memory processes
8. process server reboots
9. Read and gather logs from the server(linux/windows)
10. Forward logs to remote syslog server/api server/mqtt message bus
