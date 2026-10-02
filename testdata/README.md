# Test data

`emoji-subset.ttf` is Noto Color Emoji (`fonts/notoemoji`, a module of its
own) cut down with fontTools' `pyftsubset` to the characters the emoji tests
draw, so the tests check color glyphs and joined sequences without the full
10 MB font. Regenerate it after adding emoji to a test:

    pyftsubset ../fonts/notoemoji/NotoColorEmoji.ttf --layout-features='*' \
        --unicodes=U+0020-007E,<the characters the tests use> \
        --output-file=emoji-subset.ttf

License: SIL OFL, in `emoji-subset-OFL.txt`.
