$ORIGIN gslb.example.com.
$TTL 3600

@ IN SOA ns1.gslb.example.com. admin.gslb.example.com. (
    2026021701 ; serial
    3600       ; refresh
    900        ; retry
    604800     ; expire
    300        ; minimum TTL
)

@ IN NS ns1.gslb.example.com.
