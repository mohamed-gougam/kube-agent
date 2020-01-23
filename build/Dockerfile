  
FROM nginx:1.17.4

RUN ln -sf /proc/1/fd/1 /var/log/nginx/access.log \
	&& ln -sf /proc/1/fd/2 /var/log/nginx/error.log

RUN mkdir -p /var/lib/nginx \
	&& mkdir -p /etc/nginx/secrets

COPY kube-agent internal/configuration/version1/nginx.tcpserver.tmpl /

ENTRYPOINT ["/kube-agent"]
