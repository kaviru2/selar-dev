"""Bounded, network-free WordprocessingML snapshot adapter."""
from io import BytesIO
import zipfile
from defusedxml.ElementTree import fromstring

W = '{http://schemas.openxmlformats.org/wordprocessingml/2006/main}'


def validated_document(data):
    if len(data) > 10 << 20:
        raise ValueError('DOCX exceeds 10 MB')
    try:
        with zipfile.ZipFile(BytesIO(data)) as archive:
            infos = archive.infolist()
            if len(infos) > 1000 or len({i.filename for i in infos}) != len(infos):
                raise ValueError('DOCX has too many or duplicate entries')
            total = 0
            parts = {}
            for item in infos:
                name = item.filename
                lower = name.lower()
                if name.startswith('/') or '\\' in name or '..' in name.split('/') or item.flag_bits & 1:
                    raise ValueError('unsafe DOCX archive entry')
                if any(s in lower for s in ('vba', 'macros', 'comments', 'footnotes', 'endnotes')):
                    raise ValueError('DOCX macros, comments and footnotes/endnotes are unsupported; import an explicitly reviewed PDF instead')
                total += item.file_size
                if item.file_size > 8 << 20 or total > 32 << 20 or item.file_size > max(1, item.compress_size) * 100:
                    raise ValueError('DOCX decompression limit exceeded')
                # Read every entry through a bound and verify CRC; never extract paths.
                with archive.open(item) as stream:
                    value = stream.read((8 << 20) + 1)
                if len(value) != item.file_size or len(value) > 8 << 20:
                    raise ValueError('invalid DOCX entry size')
                if name in {'[Content_Types].xml', '_rels/.rels', 'word/document.xml'}:
                    parts[name] = value
            types = fromstring(parts['[Content_Types].xml'])
            main_type = 'application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml'
            if not any(n.get('PartName') == '/word/document.xml' and n.get('ContentType') == main_type for n in types):
                raise ValueError('not a standard DOCX document')
            relationships = fromstring(parts['_rels/.rels'])
            if not any(n.get('Type', '').endswith('/officeDocument') and n.get('Target') == 'word/document.xml' and n.get('TargetMode') != 'External' for n in relationships):
                raise ValueError('DOCX missing internal document relationship')
            root = fromstring(parts['word/document.xml'])
            if root.tag != W+'document':
                raise ValueError('invalid WordprocessingML root')
            unsupported = {'ins', 'del', 'moveFrom', 'moveTo', 'commentReference', 'footnoteReference', 'endnoteReference', 'altChunk'}
            if any(n.tag.startswith(W) and n.tag[len(W):] in unsupported for n in root.iter()):
                raise ValueError('DOCX tracked changes, comments, footnotes and alternate content are unsupported; export a reviewed PDF')
            return root
    except (zipfile.BadZipFile, KeyError) as exc:
        raise ValueError('invalid DOCX OOXML package') from exc


def docx_blocks(data):
    root = validated_document(data)
    body = root.find(W+'body')
    if body is None:
        raise ValueError('DOCX missing document body')
    blocks = []
    def text(node):
        return ''.join(n.text or '' if n.tag == W+'t' else '\t' if n.tag == W+'tab' else '\n' if n.tag in {W+'br', W+'cr'} else '' for n in node.iter())
    for body_index, node in enumerate(body):
        metadata = {'literal': True}
        if node.tag == W+'p':
            value = text(node)
            style = node.find(W+'pPr/'+W+'pStyle')
            heading = style is not None and style.get(W+'val', '').lower().startswith('heading')
            kind = 'heading' if heading else 'paragraph'
        elif node.tag == W+'tbl':
            rows = [[ '\n'.join(text(p) for p in cell.findall(W+'p')) for cell in row.findall(W+'tc')] for row in node.findall(W+'tr')]
            value = '\n'.join('\t'.join(row) for row in rows)
            kind = 'table'
            metadata['rows'] = rows
        else:
            continue
        if not value.strip():
            continue
        index = len(blocks)
        blocks.append({'kind': kind, 'text': value, 'block_index': index, 'metadata': metadata,
                       'locator': {'kind': 'docx', 'part': 'word/document.xml', 'body_index': body_index, 'block_index': index}})
    if not blocks:
        raise ValueError('DOCX has no extractable text')
    return blocks
