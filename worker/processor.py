import os
from docling.document_converter import DocumentConverter

class CodiceParser:
    def __init__(self):
        # Inicializa o conversor pesado uma única vez na memória
        print("🧠 Inicializando o motor do Docling...")
        self.converter = DocumentConverter()

    def extract_to_markdown(self, file_path):
        """Converte o PDF/EPUB em Markdown estruturado."""
        if not os.path.exists(file_path):
            raise FileNotFoundError(f"Arquivo não encontrado: {file_path}")

        print(f"⚙️ Lendo e extraindo blocos de texto...")
        
        # O Docling faz a mágica do OCR e análise de layout aqui
        result = self.converter.convert(file_path)
        
        # Exporta o resultado para a sintaxe Markdown
        md_content = result.document.export_to_markdown()
        
        # É aqui que você conectará seus parsers customizados no futuro
        # md_content = self.meu_parser_de_limpeza(md_content)
        
        return md_content