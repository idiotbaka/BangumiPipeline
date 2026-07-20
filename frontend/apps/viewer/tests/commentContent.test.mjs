import assert from 'node:assert/strict'
import test from 'node:test'

import { parseCommentContent } from '../src/commentContent.ts'

test('parses supported Bangumi comment markup without HTML injection', () => {
  const nodes = parseCommentContent(
    '正文(bgm24)\n[s]删除[/s][mask]剧透[/mask][img]https://i.vgy.me/hs6IVG.jpg[/img]',
    { '(bgm24)': '/api/bangumi-smiles/bgm24' },
  )

  assert.deepEqual(nodes, [
    { type: 'text', value: '正文' },
    { type: 'smile', code: '(bgm24)', url: '/api/bangumi-smiles/bgm24' },
    { type: 'text', value: '\n' },
    { type: 'strike', children: [{ type: 'text', value: '删除' }] },
    { type: 'mask', children: [{ type: 'text', value: '剧透' }] },
    { type: 'image', url: 'https://i.vgy.me/hs6IVG.jpg' },
  ])
})

test('filters missing smiles, unknown tags, and unsafe external images', () => {
  const nodes = parseCommentContent(
    '(bgm999)[unknown]保留文字[/unknown][img]javascript:alert(1)[/img][img]https://u:p@example.com/a.png[/img]',
    {},
  )

  assert.deepEqual(nodes, [{ type: 'text', value: '保留文字' }])
})

test('parses sized image tags and bounds their display hint', () => {
  assert.deepEqual(
    parseCommentContent('[img=176,99]https://lain.bgm.tv/pic/photo/l/19/57/138463_prZGO.jpg[/img]', {}),
    [{
      type: 'image',
      url: 'https://lain.bgm.tv/pic/photo/l/19/57/138463_prZGO.jpg',
      width: 176,
      height: 99,
    }],
  )

  assert.deepEqual(
    parseCommentContent('[img=1760,990]https://example.com/large.jpg[/img]', {}),
    [{ type: 'image', url: 'https://example.com/large.jpg', width: 360, height: 203 }],
  )
})

test('removes malformed supported tags while preserving readable text', () => {
  const nodes = parseCommentContent('[s]未闭合[/mask]正文', {})
  assert.deepEqual(nodes, [{ type: 'text', value: '未闭合正文' }])
})
