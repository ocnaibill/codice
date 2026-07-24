# Códice 📚

Um sistema completo, moderno e open-source para gerenciamento e consumo de bibliotecas digitais (Homelab). O Códice suporta livros de ficção (EPUB), coleções de mangás/HQs (CBZ) e possui uma esteira especializada com Full-Text Search para PDFs de estudos e artigos acadêmicos.

## 🏗️ Arquitetura

O Códice é construído com foco em performance, resiliência e baixo consumo de recursos, ideal para rodar em servidores locais (Homelabs).

*   **Backend (API & OPDS):** Go (Golang)
*   **Worker de Extração:** Python + Docling (Processamento assíncrono de PDFs)
*   **Fila de Mensagens:** Redis Streams
*   **Banco de Dados:** PostgreSQL (com índices GIN para buscas Full-Text)
*   **Frontend:** React (Vite) + Tailwind CSS + Zustand

## 🚀 Status do Projeto

Atualmente em fase de **fundação estrutural**. Consulte as [Issues](https://github.com/ocnaibill/codice/issues) para ver o que está sendo desenvolvido.

## 📄 Licença

Este projeto está licenciado sob a **GNU AGPLv3** - veja o arquivo [LICENSE](LICENSE) para detalhes. O Códice é copyleft: sinta-se livre para hospedar, usar e modificar, desde que mantenha o código-fonte de qualquer modificação público.