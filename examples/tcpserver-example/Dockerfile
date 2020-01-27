FROM nginx:1.16.1-alpine

# We need to give all user groups write permission on folder /var/cache/nginx/ and file /var/run/nginx.pid.
# So users with random uid will be able to run NGINX.
RUN chmod -R a+w /var/cache/nginx/ \
        && touch /var/run/nginx.pid \
        && chmod a+w /var/run/nginx.pid \
        && rm /etc/nginx/nginx.conf \
        && rm /etc/nginx/conf.d/*

COPY nginx.conf /etc/nginx/
COPY hello-tcp.conf /etc/nginx/conf.d/tcp/
EXPOSE 12345
USER nginx