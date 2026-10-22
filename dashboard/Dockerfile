FROM python:3.12-alpine

WORKDIR /app
RUN pip install --no-cache-dir Flask==3.1.1 gunicorn==23.0.0
COPY app.py index.html ./

EXPOSE 8080
CMD ["gunicorn", "--bind", "0.0.0.0:8080", "--workers", "2", "--timeout", "300", "app:app"]
