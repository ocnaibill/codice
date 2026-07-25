import os
from docling.document_converter import DocumentConverter

class CodiceParser:
    def __init__(self, allowed_dirs=None):
        # Inicializa o conversor pesado uma única vez na memória
        print("🧠 Inicializando o motor do Docling...")
        self.converter = DocumentConverter()
        
        # Define diretórios permitidos para processamento de arquivos
        if allowed_dirs is None:
            base_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
            self.allowed_dirs = [
                os.path.abspath(os.path.join(base_dir, "backend", "uploads")),
                os.path.abspath(os.path.join(base_dir, "uploads"))
            ]
        else:
            self.allowed_dirs = [os.path.abspath(d) for d in allowed_dirs]

    def is_safe_path(self, file_path):
        """Verifica se o caminho do arquivo está estritamente dentro dos diretórios permitidos."""
        abs_target = os.path.abspath(file_path)
        for allowed in self.allowed_dirs:
            if abs_target.startswith(allowed + os.sep) or abs_target == allowed:
                return True
        return False

    def extract_to_markdown(self, file_path):
        """Converte o PDF/EPUB em Markdown estruturado."""
        if not os.path.exists(file_path):
            raise FileNotFoundError(f"Arquivo não encontrado: {file_path}")

        if not self.is_safe_path(file_path):
            raise ValueError(f"Acesso negado a arquivo fora do diretório de uploads: {file_path}")

        print(f"⚙️ Lendo e extraindo blocos de texto...")
        
        # O Docling faz a mágica do OCR e análise de layout aqui
        result = self.converter.convert(file_path)
        
        # Exporta o resultado para a sintaxe Markdown
        md_content = result.document.export_to_markdown()
        
        return md_content