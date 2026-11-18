import io
import os

import clip
import torch
from fastapi import FastAPI, File, HTTPException, UploadFile
from PIL import Image, UnidentifiedImageError

MODEL_NAME = "ViT-B/32"
MODEL_VERSION = "openai-clip-vit-b-32"
MAX_BYTES = 10 * 1024 * 1024
app = FastAPI(docs_url=None, redoc_url=None)
device = "cpu"
enabled = os.getenv("IMAGE_SEARCH_ENABLED", "false").lower() == "true"
model, preprocess = (None, None)
if enabled:
    model, preprocess = clip.load(MODEL_NAME, device=device)
    model.eval()


@app.get("/health")
def health():
    return {"status": "ok" if enabled else "disabled", "model": MODEL_VERSION}


@app.post("/v1/embeddings/images")
async def image_embedding(image: UploadFile = File(...)):
    if not enabled:
        raise HTTPException(status_code=503, detail="image search disabled")
    raw = await image.read(MAX_BYTES + 1)
    if not raw or len(raw) > MAX_BYTES:
        raise HTTPException(status_code=400, detail="invalid image size")
    try:
        decoded = Image.open(io.BytesIO(raw))
        if decoded.format not in {"JPEG", "PNG", "WEBP", "GIF"}:
            raise HTTPException(status_code=400, detail="unsupported image type")
        decoded.seek(0)
        if decoded.width > 8000 or decoded.height > 8000 or decoded.width * decoded.height > 40_000_000:
            raise HTTPException(status_code=400, detail="image dimensions too large")
        decoded = decoded.convert("RGB")
        tensor = preprocess(decoded).unsqueeze(0).to(device)
    except (UnidentifiedImageError, OSError, ValueError):
        raise HTTPException(status_code=400, detail="invalid image")
    with torch.inference_mode():
        features = model.encode_image(tensor).float()
        features = features / features.norm(dim=-1, keepdim=True).clamp(min=1e-12)
    return {"model": MODEL_VERSION, "vector": features[0].tolist()}
