FROM python:3.11-slim
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends git && rm -rf /var/lib/apt/lists/*
COPY requirements.txt ./
RUN pip install --no-cache-dir --index-url https://download.pytorch.org/whl/cpu --extra-index-url https://pypi.org/simple torch==2.7.1+cpu torchvision==0.22.1+cpu \
    && pip install --no-cache-dir -r requirements.txt
COPY app.py ./
RUN useradd --create-home --uid 10001 appuser && mkdir -p /model-cache && chown -R appuser:appuser /app /model-cache
USER appuser
ENV TORCH_HOME=/model-cache HOME=/model-cache
EXPOSE 8090
CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8090"]
