import os
import subprocess
import sys
import venv
from pathlib import Path

def setup_voice_env(base_dir):
    env_path = Path(base_dir) / "voice_env"
    print(f"Creating virtual environment at {env_path}...")
    
    if not env_path.exists():
        venv.create(env_path, with_pip=True)
    
    # Path to the virtual environment's pip
    if os.name == "nt":
        pip_exe = env_path / "Scripts" / "pip.exe"
        python_exe = env_path / "Scripts" / "python.exe"
    else:
        pip_exe = env_path / "bin" / "pip"
        python_exe = env_path / "bin" / "python"

    print("Installing heavy dependencies (VoxCPM, Whisper, FastAPI, PyTorch)...")
    print("This may take several minutes depending on your internet connection.")
    
    dependencies = [
        "fastapi",
        "uvicorn[standard]",
        "openai-whisper",
        "voxcpm",
        "requests",
        "pydub",
        "python-multipart"
    ]
    
    try:
        subprocess.check_call([str(pip_exe), "install"] + dependencies)
        print("\n[✓] Voice environment successfully configured.")
    except subprocess.CalledProcessError as e:
        print(f"\n[✗] Error installing dependencies: {e}")
        sys.exit(1)

if __name__ == "__main__":
    state_dir = os.path.expanduser("~/.assistclaw")
    if len(sys.argv) > 1:
        state_dir = sys.argv[1]
    
    setup_voice_env(state_dir)
