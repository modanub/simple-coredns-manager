$ORIGIN example.com.
$TTL 3600

@ IN SOA ns1.example.com. admin.example.com. (
    2026021701 ; serial
    3600       ; refresh
    900        ; retry
    604800     ; expire
    300        ; minimum TTL
)

@ IN NS ns1.example.com.

; A records
app IN A 192.168.1.10
api IN A 192.168.1.11
db  IN A 192.168.1.12
