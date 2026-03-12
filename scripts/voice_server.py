import os
import io
import sys
import torch
import whisper
import uvicorn
from fastapi import FastAPI, UploadFile, File, Form
from fastapi.responses import Response
from voxcpm import VoxCPM # Assuming standard import based on voxcpm docs

app = FastAPI(title="AssistClaw Voice Microservice")

# Global models
stt_model = None
tts_model = None

@app.on_event("startup")
def load_models():
    global stt_model, tts_model
    device = "cuda" if torch.cuda.is_available() else "cpu"
    print(f"Loading Whisper STT on {device}...")
    stt_model = whisper.load_model("base", device=device)
    
    print(f"Loading VoxCPM TTS...")
    # This is a placeholder for actual VoxCPM 1.5 loading logic
    # Based on the description: end-to-end diffusion AR
    # Typically: model = VoxCPM.from_pretrained(...)
    try:
        tts_model = VoxCPM.from_pretrained("openbmb/VoxCPM-1.5")
    except:
        print("VoxCPM load failed (placeholder mode)")

from pydub import AudioSegment

@app.post("/stt")
async def speech_to_text(audio: UploadFile = File(...), format: str = Form("wav")):
    audio_bytes = await audio.read()
    
    # Use pydub to handle various formats (including concat opus packets if possible)
    # Note: Discord Opus packets without Ogg headers might need special handling.
    # For now, we assume we send WAV/MP3 from Go or a file with headers.
    
    audio_stream = io.BytesIO(audio_bytes)
    audio_seg = AudioSegment.from_file(audio_stream, format=format)
    audio_seg.export("temp_stt.wav", format="wav")
    
    result = stt_model.transcribe("temp_stt.wav")
    return {"text": result["text"]}

@app.post("/tts")
async def text_to_speech(text: str = Form(...), reference_audio: UploadFile = File(None), output_format: str = Form("wav")):
    print(f"Synthesizing: {text}")
    
    # Conceptual VoxCPM synthesis
    # audio_data = tts_model.generate(text, reference_audio=...)
    
    # Dummy PCM for now
    dummy_wav = AudioSegment.silent(duration=1000) # 1 sec silence
    
    buffer = io.BytesIO()
    dummy_wav.export(buffer, format=output_format)
    
    return Response(content=buffer.getvalue(), media_type=f"audio/{output_format}")
import base64

@app.post("/tts/discord")
async def text_to_speech_discord(text: str = Form(...)):
    # 1. Synthesize to PCM (dummy for now)
    dummy_wav = AudioSegment.silent(duration=1000) # 1 sec silence
    
    # 2. Convert to Discord-compatible Opus (48kHz, mono)
    # This requires ffmpeg with libopus
    dummy_wav = dummy_wav.set_frame_rate(48000).set_channels(1)
    
    # Slice into 20ms chunks
    chunks = []
    for i in range(0, len(dummy_wav), 20):
        chunk = dummy_wav[i:i+20]
        # Export this 20ms chunk to opus
        buf = io.BytesIO()
        chunk.export(buf, format="opus") # Note: pydub might need a specific format string or just 'ogg'
        chunks.append(base64.b64encode(buf.getvalue()).decode('utf-8'))
        
    return {"packets": chunks}

@app.get("/")
async def health_check():
    return {"status": "ok", "stt": stt_model is not None, "tts": tts_model is not None}

if __name__ == "__main__":
    port = int(os.getenv("VOICE_PORT", 11000))
    uvicorn.run(app, host="127.0.0.1", port=port)
