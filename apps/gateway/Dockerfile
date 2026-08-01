FROM nginx:1.25-alpine

# Remove default nginx config
RUN rm /etc/nginx/conf.d/default.conf

# Copy custom nginx config and includes
COPY nginx.conf /etc/nginx/nginx.conf
COPY includes/ /etc/nginx/includes/

# Copy avatar files
COPY avatars /var/www/avatars/

# Ensure nginx can write to required directories as non-root
RUN chown -R nginx:nginx /var/cache/nginx /var/log/nginx /var/run && \
    chmod -R 755 /var/cache/nginx /var/log/nginx /var/run

# Run as non-root (match other containers' security posture)
USER nginx

EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:8080/health || exit 1

CMD ["nginx", "-g", "daemon off;"]
