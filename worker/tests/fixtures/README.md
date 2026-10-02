Arquivos reais, pequenos, para os testes dos leitores de metadados (#25). Foram gerados com o `ffmpeg`
(4 s de silêncio, mono, 16 kb/s) a partir de um arquivo de metadados com três capítulos (0 s, 1 s e 2,5 s),
um comentário e uma descrição:

    ffmpeg -f lavfi -i anullsrc=r=22050:cl=mono -i meta.txt -map_metadata 1 -t 4 -c:a aac -b:a 16k -f ipod capitulos.m4b
    ffmpeg -f lavfi -i anullsrc=r=22050:cl=mono -i meta.txt -map_metadata 1 -t 4 -c:a libmp3lame -b:a 16k -id3v2_version 3 capitulos.mp3

O M4B guarda os capítulos no formato do `chpl`; o MP3, em quadros `CHAP` com `TIT2`, e o comentário e a descrição
em quadros `TXXX` (não `COMM`).

`silencio.flac`: 2 s de silêncio, sem etiquetas além do codificador; os testes acrescentam capítulos
(`CHAPTER001=00:00:00.000`, `CHAPTER001NAME=...`) e uma descrição com o `mutagen`.
