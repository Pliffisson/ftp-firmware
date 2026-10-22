import os
import hashlib
import json
import time
import stat
import tempfile
import unicodedata
from pathlib import Path

from flask import Flask, jsonify, request, send_file
from werkzeug.exceptions import RequestEntityTooLarge


app = Flask(__name__)
app.config["MAX_CONTENT_LENGTH"] = 2 * 1024 * 1024 * 1024
FIRMWARE_DIR = Path("/srv/tftp")
LOGO_PATH = Path("/srv/branding/logo.png")
TRANSFER_DIR = Path("/srv/transfers")


def valid_name(name):
    return (
        bool(name)
        and not name.startswith(".")
        and len(name.encode("utf-8")) <= 255
        and "/" not in name
        and "\\" not in name
        and not any(unicodedata.category(char).startswith("C") for char in name)
    )


def clean_upload_name(raw_name):
    # Alguns navegadores enviam o caminho completo ou caracteres invisíveis.
    basename = (raw_name or "").replace("\\", "/").rsplit("/", 1)[-1]
    normalized = unicodedata.normalize("NFKC", basename)
    return "".join(
        char for char in normalized
        if not unicodedata.category(char).startswith("C")
    ).strip()


@app.get("/")
def index():
    return send_file("index.html")


@app.get("/api/logo")
def get_logo():
    if not LOGO_PATH.is_file():
        return "", 404
    response = send_file(LOGO_PATH, mimetype="image/png")
    response.headers["Cache-Control"] = "no-store"
    return response


@app.get("/api/files")
def list_files():
    files = []
    for path in FIRMWARE_DIR.iterdir():
        if not valid_name(path.name) or path.is_symlink() or not path.is_file():
            continue
        info = path.stat()
        files.append({"name": path.name, "size": info.st_size, "modified": info.st_mtime})
    files.sort(key=lambda item: item["modified"], reverse=True)
    return jsonify(files)


@app.get("/api/transfers/<name>")
def get_transfer(name):
    if not valid_name(name):
        return jsonify(error="Nome de arquivo inválido."), 400
    path = TRANSFER_DIR / (hashlib.sha256(name.encode("utf-8")).hexdigest() + ".json")
    try:
        with path.open("r", encoding="utf-8") as source:
            result = json.load(source)
    except (FileNotFoundError, json.JSONDecodeError):
        return "", 204
    result["age_ms"] = max(0, int(time.time() * 1000) - result.get("started_at_ms", 0))
    return jsonify(result)


@app.post("/api/files")
def upload_file():
    uploaded = request.files.get("file")
    if uploaded is None:
        return jsonify(error="Selecione um arquivo antes de enviar."), 400
    filename = clean_upload_name(uploaded.filename)
    if not valid_name(filename):
        return jsonify(error="Não foi possível usar o nome deste arquivo."), 400

    destination = FIRMWARE_DIR / filename
    if destination.exists() or destination.is_symlink():
        return jsonify(error="Já existe um arquivo com esse nome."), 409

    descriptor, temp_name = tempfile.mkstemp(prefix=".upload-", dir=FIRMWARE_DIR)
    try:
        with os.fdopen(descriptor, "wb") as output:
            while chunk := uploaded.stream.read(1024 * 1024):
                output.write(chunk)
        os.chmod(temp_name, stat.S_IRUSR | stat.S_IWUSR | stat.S_IRGRP | stat.S_IROTH)
        # A criação exclusiva evita substituir um firmware enviado simultaneamente.
        os.link(temp_name, destination)
    except FileExistsError:
        return jsonify(error="Já existe um arquivo com esse nome."), 409
    finally:
        os.unlink(temp_name)

    return jsonify(name=destination.name, size=destination.stat().st_size), 201


@app.delete("/api/files/<name>")
def delete_file(name):
    if not valid_name(name):
        return jsonify(error="Nome de arquivo inválido."), 400
    path = FIRMWARE_DIR / name
    if path.is_symlink() or not path.is_file():
        return jsonify(error="Arquivo não encontrado."), 404
    try:
        path.unlink()
    except FileNotFoundError:
        return jsonify(error="Arquivo não encontrado."), 404
    return "", 204


@app.errorhandler(RequestEntityTooLarge)
def too_large(_error):
    return jsonify(error="Arquivo maior que o limite de 2 GB."), 413
