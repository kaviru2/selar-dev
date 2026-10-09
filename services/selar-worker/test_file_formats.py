import hashlib
import io
import zipfile


def docx_bytes(body='<w:p><w:r><w:t>co- operation evidence.</w:t></w:r></w:p>', extra=None):
    output = io.BytesIO()
    with zipfile.ZipFile(output, 'w', zipfile.ZIP_DEFLATED) as archive:
        archive.writestr('[Content_Types].xml', '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>')
        archive.writestr('_rels/.rels', '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>')
        archive.writestr('word/document.xml', '<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>'+body+'</w:body></w:document>')
        for name, data in (extra or {}).items():
            archive.writestr(name, data)
    return output.getvalue()


def test_docx_ordered_blocks_original_hash_and_locators(tmp_path):
    data = docx_bytes('<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Topic</w:t></w:r></w:p><w:p><w:r><w:t>co- operation evidence.</w:t></w:r></w:p><w:tbl><w:tr><w:tc><w:p><w:r><w:t>Cell evidence</w:t></w:r></w:p></w:tc></w:tr></w:tbl>')
    path = tmp_path/'original.docx'; path.write_bytes(data)
    source = asyncio.run(ingestion.extract_source(source_type='docx',doc_id='fixture',word_limit=100,merge_bboxes=None,file_path=str(path),title=path.name))
    assert [b['kind'] for b in source.blocks] == ['heading', 'paragraph', 'table']
    assert source.blocks[1]['text'] == 'co- operation evidence.'
    assert source.blocks[2]['text'] == 'Cell evidence'
    assert source.content_hash == hashlib.sha256(data).hexdigest()
    assert source.blocks[2]['locator']['body_index'] == 2
    assert source.metadata['original_filename'] == path.name
    assert source.extractor_version


import asyncio
import pytest
import ingestion
from ingestion import extract_text, markdown_blocks


@pytest.fixture(autouse=True)
def isolated_storage(tmp_path, monkeypatch):
    from storage import LocalStorage
    monkeypatch.setattr(ingestion, 'get_storage', lambda: LocalStorage(str(tmp_path)))


@pytest.mark.parametrize('kind,filename,data', [('markdown', 'notes.md', b'# Topic\n\nco- operation.'), ('txt', 'notes.txt', b'# literal heading\nco- operation.')])
def test_stored_text_file_original_hash_and_literal_txt(tmp_path, kind, filename, data):
    path = tmp_path / filename
    path.write_bytes(data)
    source = asyncio.run(ingestion.extract_source(source_type=kind, doc_id='fixture', word_limit=100,
        merge_bboxes=None, file_path=str(path), title=filename))
    assert source.content_hash == hashlib.sha256(data).hexdigest()
    assert source.metadata['original_filename'] == filename
    assert 'co- operation.' in source.chunks[-1]['text']
    if kind == 'txt':
        assert source.blocks[0]['kind'] == 'paragraph'
        assert source.blocks[0]['text'].startswith('# literal')



@pytest.mark.parametrize('body,extra', [
    ('<w:p><w:ins><w:r><w:t>changed</w:t></w:r></w:ins></w:p>', {}),
    ('<w:p><w:r><w:t>Evidence</w:t></w:r></w:p>', {'word/comments.xml': '<comments/>'}),
    ('<w:p><w:r><w:t>Evidence</w:t></w:r></w:p>', {'word/footnotes.xml': '<footnotes/>'}),
    ('<w:p><w:r><w:t>Evidence</w:t></w:r></w:p>', {'word/vbaProject.bin': 'macro'}),
    ('<w:p><w:r><w:t>Evidence</w:t></w:r></w:p>', {'../escape': 'bad'}),
    ('<w:p><w:r><w:t>Evidence</w:t></w:r></w:p>', {'word/bomb': 'A' * 1000000}),
])
def test_docx_rejects_unsafe_or_unrepresented_evidence(body, extra):
    from document_formats import docx_blocks
    with pytest.raises(ValueError):
        docx_blocks(docx_bytes(body, extra))


@pytest.mark.parametrize('missing', ['[Content_Types].xml', '_rels/.rels'])
def test_docx_requires_ooxml_structure(missing):
    from document_formats import docx_blocks
    original = zipfile.ZipFile(io.BytesIO(docx_bytes()))
    output = io.BytesIO()
    with zipfile.ZipFile(output, 'w') as archive:
        for name in original.namelist():
            if name != missing:
                archive.writestr(name, original.read(name))
    with pytest.raises(ValueError):
        docx_blocks(output.getvalue())


def test_docx_rejects_excessive_xml_depth():
    from document_formats import docx_blocks
    body = '<w:p>' + '<w:r>' * 129 + '<w:t>Evidence</w:t>' + '</w:r>' * 129 + '</w:p>'
    with pytest.raises(ValueError, match='XML.*limit'):
        docx_blocks(docx_bytes(body))


def test_docx_rejects_excessive_xml_nodes():
    from document_formats import fromstring
    with pytest.raises(ValueError, match='XML.*limit'):
        fromstring(b'<root>' + b'<n/>' * 100001 + b'</root>')


def test_markdown_line_slices_follow_commonmark_newlines():
    source = 'first\vstill first\n\n| A | B |\n| --- | --- |\n| one | two |\n'
    blocks = markdown_blocks(source)
    assert blocks[-1]['text'] == '| A | B |\n| --- | --- |\n| one | two |\n'
    assert blocks[-1]['locator']['line_start'] == 3


def test_markdown_ordered_list_marker_and_heading_level():
    blocks = markdown_blocks('### Third\n\n7. ordered witness\n8. next witness')
    assert blocks[0]['metadata']['level'] == 3
    assert blocks[1]['metadata']['marker'] == '7.'
    assert blocks[2]['metadata']['marker'] == '8.'


def test_markdown_fenced_code_and_table_preserve_source_structure():
    text = '# Evidence\n\nco- operation stays distinct.\n\n```python\nx = "a  b"\n```\n\n| A | B |\n| --- | --- |\n| one | two |\n'
    source = extract_text(text, 'Notes', 100)
    assert [b['kind'] for b in source.blocks] == ['heading', 'paragraph', 'code', 'table']
    assert source.blocks[2]['text'] == 'x = "a  b"\n'
    assert 'co- operation' in source.chunks[0]['text']
    assert source.content_hash == hashlib.sha256(text.encode()).hexdigest()
    for block in source.blocks:
        assert block['locator']['line_start'] >= 1
        assert block['locator']['line_end'] >= block['locator']['line_start']
