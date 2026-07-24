import os
import time
import redis
from dotenv import load_dotenv

# 1. Loads variables from the .env file located at the project root
# The worker is inside the /worker folder, so the .env is one level up
load_dotenv(dotenv_path="../.env")

# 2. Configures the Redis connection
REDIS_URL = os.getenv("REDIS_URL", "redis://localhost:6379/0")

r = redis.from_url(
    REDIS_URL, 
    decode_responses=True,
    socket_timeout=10.0,         # Deve ser estritamente MAIOR que o tempo do 'block' (5s)
    socket_connect_timeout=5.0,  # Tempo máximo para tentar a primeira conexão
    socket_keepalive=True,       # Avisa a rede do Windows/Docker para não derrubar a conexão ociosa
    retry_on_timeout=True        # Tenta reconectar silenciosamente se houver oscilação
)

STREAM_NAME = 'ingestion_tasks'
GROUP_NAME = 'python_workers'
CONSUMER_NAME = 'worker_1'

def setup_redis_stream():
    """Creates the Consumer Group in Redis, if it does not exist."""
    try:
        r.ping()
        print("✅ Successfully connected to Redis!")
        r.xgroup_create(STREAM_NAME, GROUP_NAME, id='0', mkstream=True)
        print(f"📦 Consumer group '{GROUP_NAME}' configured.")
    except redis.exceptions.ResponseError as e:
        if "BUSYGROUP" not in str(e):
            print(f"❌ Error creating group: {e}")
    except redis.exceptions.ConnectionError:
        print("❌ Could not connect to Redis. Is the container running?")
        exit(1)

def listen_for_tasks():
    """Main loop that listens for new tasks in the queue."""
    setup_redis_stream()
    print("⏳ Python Worker waiting for PDFs in the queue...")

    while True:
        try:
            # Requests new messages ('>') and waits up to 5 seconds
            messages = r.xreadgroup(
                groupname=GROUP_NAME, 
                consumername=CONSUMER_NAME, 
                streams={STREAM_NAME: '>'}, 
                block=5000, 
                count=1
            )

            if not messages:
                continue

            for stream, message_list in messages:
                for message_id, data in message_list:
                    file_path = data.get('file_path')
                    work_id = data.get('work_id')
                    
                    print(f"\n📥 New task received! ID: {message_id}")
                    print(f"   Processing file: {file_path}")
                    
                    # TODO: The extract_pdf_data() function using Docling will go here
                    time.sleep(2) # Simulating work
                    
                    print(f"✅ Task {message_id} completed. Acknowledging Redis...")
                    r.xack(STREAM_NAME, GROUP_NAME, message_id)

        except Exception as e:
            print(f"⚠️ Unexpected error: {e}")
            time.sleep(2)

if __name__ == "__main__":
    listen_for_tasks()