import React, { useState, useEffect, useRef, useCallback } from 'react'
import './App.css'

const API_BASE = ''

const VIEW_LABELS = {
  front: '前视图',
  rear: '后视图',
  left: '左视图',
  right: '右视图',
  top: '顶视图',
}

const VIEW_COLORS = {
  front: '#4A90D9',
  rear: '#E74C3C',
  left: '#2ECC71',
  right: '#F39C12',
  top: '#9B59B6',
}

function groupModels(models) {
  const groups = {}
  for (const m of models) {
    let series = m.id
    if (series.startsWith('model3')) series = 'model3'
    else if (series.startsWith('modely')) series = 'modely'
    else if (series.startsWith('models')) series = 'models'
    else if (series.startsWith('modelx')) series = 'modelx'
    if (!groups[series]) groups[series] = { id: series, models: [] }
    groups[series].models.push(m)
  }
  return groups
}

const SERIES_ORDER = ['cybertruck', 'model3', 'modely', 'models', 'modelx']
const SERIES_LABELS = { cybertruck:'Cybertruck', model3:'Model 3', modely:'Model Y', models:'Model S', modelx:'Model X' }
const SERIES_ICONS = { cybertruck:'🛻', model3:'🚗', modely:'🚙', models:'🏎️', modelx:'🚘' }

export default function App() {
  const [models, setModels] = useState([])
  const [modelGroups, setModelGroups] = useState({})
  const [selectedModel, setSelectedModel] = useState('')
  const [modelDetails, setModelDetails] = useState(null)
  const [images, setImages] = useState({})
  const [adjustments, setAdjustments] = useState({})
  const [templateUrl, setTemplateUrl] = useState('')
  const [rendering, setRendering] = useState(false)
  const [resultUrl, setResultUrl] = useState(null)
  const [dragOver, setDragOver] = useState(null)
  const [error, setError] = useState('')

  // Drag state for overlay adjustment
  const [selectedView, setSelectedView] = useState(null)
  const dragging = useRef(null) // { viewName, startX, startY, origOffX, origOffY }
  const previewRect = useRef(null)
  const resultUrlRef = useRef(null)
  const previewRef = useRef(null)

  useEffect(() => {
    fetch(`${API_BASE}/api/models`)
      .then(r => r.json())
      .then(data => {
        setModels(data)
        setModelGroups(groupModels(data))
        const firstGroupId = SERIES_ORDER.find(k => groupModels(data)[k]?.models.length > 0)
        if (firstGroupId) {
          setSelectedModel(groupModels(data)[firstGroupId].models[0].id)
        }
      })
      .catch(e => setError('加载车型列表失败: ' + e.message))
  }, [])

  useEffect(() => {
    if (!selectedModel) return
    if (resultUrlRef.current) URL.revokeObjectURL(resultUrlRef.current)
    resultUrlRef.current = null
    setResultUrl(null)
    setTemplateUrl(`${API_BASE}/api/models/${selectedModel}/template`)
    fetch(`${API_BASE}/api/models/${selectedModel}`)
      .then(r => r.json())
      .then(data => setModelDetails(data))
      .catch(e => setError('加载车型详情失败: ' + e.message))
    setAdjustments({})
    setSelectedView(null)
  }, [selectedModel])

  const handleImageUpload = useCallback((viewName, file) => {
    setImages(prev => ({ ...prev, [viewName]: file }))
    setResultUrl(null)
    setAdjustments(prev => ({
      ...prev,
      [viewName]: prev[viewName] || { scale: 100, rotate: 0, offsetX: 0, offsetY: 0, flipH: false },
    }))
  }, [])

  const updateAdjustment = useCallback((viewName, field, value) => {
    setAdjustments(prev => ({
      ...prev,
      [viewName]: { ...prev[viewName], [field]: value },
    }))
    setResultUrl(null)
  }, [])

  const handleDrop = useCallback((e, viewName) => {
    e.preventDefault()
    setDragOver(null)
    const file = e.dataTransfer.files[0]
    if (file && file.type.startsWith('image/')) handleImageUpload(viewName, file)
  }, [handleImageUpload])

  const handleFileInput = useCallback((e, viewName) => {
    const file = e.target.files[0]
    if (file) handleImageUpload(viewName, file)
  }, [handleImageUpload])

  // --- Mouse drag on preview overlays ---
  const handleOverlayMouseDown = useCallback((e, viewName) => {
    e.stopPropagation()
    setSelectedView(viewName)
    const rect = previewRef.current?.getBoundingClientRect()
    if (!rect) return
    previewRect.current = rect
    const adj = adjustments[viewName] || { offsetX: 0, offsetY: 0 }
    dragging.current = {
      viewName,
      startX: e.clientX,
      startY: e.clientY,
      origOffX: adj.offsetX || 0,
      origOffY: adj.offsetY || 0,
    }
  }, [adjustments])

  const handleOverlayWheel = useCallback((e, viewName) => {
    e.preventDefault()
    e.stopPropagation()
    const adj = adjustments[viewName] || { scale: 100 }
    const delta = e.deltaY > 0 ? -5 : 5
    const newScale = Math.max(30, Math.min(300, (adj.scale || 100) + delta))
    updateAdjustment(viewName, 'scale', newScale)
  }, [adjustments, updateAdjustment])

  useEffect(() => {
    const handleMouseMove = (e) => {
      if (!dragging.current) return
      const { viewName, startX, startY, origOffX, origOffY } = dragging.current
      const rect = previewRect.current
      if (!rect) return
      const templateEl = previewRef.current?.querySelector('.template-image')
      if (!templateEl) return
      const tRect = templateEl.getBoundingClientRect()
      const scaleX = (modelDetails?.width || 1) / tRect.width
      const scaleY = (modelDetails?.height || 1) / tRect.height
      const dx = (e.clientX - startX) * scaleX
      const dy = (e.clientY - startY) * scaleY
      updateAdjustment(viewName, 'offsetX', Math.round(origOffX + dx))
      updateAdjustment(viewName, 'offsetY', Math.round(origOffY + dy))
    }
    const handleMouseUp = () => {
      dragging.current = null
    }
    window.addEventListener('mousemove', handleMouseMove)
    window.addEventListener('mouseup', handleMouseUp)
    return () => {
      window.removeEventListener('mousemove', handleMouseMove)
      window.removeEventListener('mouseup', handleMouseUp)
    }
  }, [modelDetails, updateAdjustment])

  const handleRender = async () => {
    if (!selectedModel || Object.keys(images).length === 0) {
      setError('请先选择车型并上传至少一张图片')
      return
    }
    setRendering(true)
    setError('')
    setResultUrl(null)

    const formData = new FormData()
    formData.append('model_id', selectedModel)
    for (const [view, file] of Object.entries(images)) {
      formData.append(view, file)
    }
    formData.append('adjustments', JSON.stringify(adjustments))

    try {
      const res = await fetch(`${API_BASE}/api/render`, {
        method: 'POST',
        body: formData,
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text)
      }
      const blob = await res.blob()
      if (resultUrlRef.current) URL.revokeObjectURL(resultUrlRef.current)
      const url = URL.createObjectURL(blob)
      resultUrlRef.current = url
      setResultUrl(url)
    } catch (e) {
      setError('渲染失败: ' + e.message)
    } finally {
      setRendering(false)
    }
  }

  const handleDownload = () => {
    if (!resultUrl) return
    const a = document.createElement('a')
    a.href = resultUrl
    a.download = `${selectedModel}_wrap.png`
    a.click()
  }

  const handleClearView = (viewName) => {
    setImages(prev => { const n = { ...prev }; delete n[viewName]; return n })
    setAdjustments(prev => { const n = { ...prev }; delete n[viewName]; return n })
    if (selectedView === viewName) setSelectedView(null)
  }

  return (
    <div className="app">
      <header className="header">
        <div className="header-content">
          <h1><span className="logo">🚗</span> Tesla Wrap Studio</h1>
          <p className="subtitle">自定义特斯拉车身贴膜设计工具 — 拖拽视图调整位置，滚轮缩放</p>
        </div>
      </header>

      <main className="main">
        {error && (
          <div className="error-banner">
            <span>⚠️ {error}</span>
            <button onClick={() => setError('')}>✕</button>
          </div>
        )}

        <section className="section" style={{ marginBottom: 12 }}>
          <label className="section-label">选择车型</label>
          <select className="model-select"
            value={selectedModel}
            onChange={e => setSelectedModel(e.target.value)}>
            {SERIES_ORDER.filter(sid => modelGroups[sid]?.models.length > 0).map(sid => {
              const group = modelGroups[sid]
              return (
                <optgroup key={sid} label={`${SERIES_ICONS[sid]} ${SERIES_LABELS[sid]}`}>
                  {group.models.map(m => (
                    <option key={m.id} value={m.id}>{m.name}</option>
                  ))}
                </optgroup>
              )
            })}
          </select>
        </section>

        <div className="workspace">
          {/* Left: upload grid */}
          <section className="section upload-panel">
            <label className="section-label">上传图片（点击或拖拽文件到对应区域）</label>
            <div className="upload-grid">
              {(modelDetails ? modelDetails.views : []).filter(v => !v.skip).map(view => {
                const adj = adjustments[view.name] || { scale: 100, rotate: 0, offsetX: 0, offsetY: 0, flipH: false }
                return (
                  <UploadBox
                    key={view.name}
                    viewName={view.name}
                    label={VIEW_LABELS[view.name] || view.name}
                    color={VIEW_COLORS[view.name] || '#666'}
                    file={images[view.name]}
                    onUpload={handleImageUpload}
                    onDrop={handleDrop}
                    onClear={() => handleClearView(view.name)}
                    dragOver={dragOver}
                    setDragOver={setDragOver}
                    adjustments={adj}
                  />
                )
              })}
            </div>
          </section>

          {/* Right: preview with draggable overlays */}
          <section className="section preview-panel">
            <label className="section-label">
              预览区域
              {selectedView && (
                <span className="selected-view-hint" style={{ color: VIEW_COLORS[selectedView] }}>
                  — 选中 {VIEW_LABELS[selectedView]}：拖拽移动 · 滚轮缩放
                  <button className="flip-btn"
                    onClick={() => updateAdjustment(selectedView, 'flipH', !(adjustments[selectedView]?.flipH || false))}>
                    {adjustments[selectedView]?.flipH ? '↔ 已翻转' : '↔ 翻转'}
                  </button>
                  <button className="reset-btn"
                    onClick={() => {
                      updateAdjustment(selectedView, 'offsetX', 0)
                      updateAdjustment(selectedView, 'offsetY', 0)
                      updateAdjustment(selectedView, 'scale', 100)
                      updateAdjustment(selectedView, 'rotate', 0)
                    }}>
                    ↺ 重置
                  </button>
                </span>
              )}
            </label>
            <div className="template-preview" ref={previewRef}>
              {resultUrl ? (
                <img src={resultUrl} alt="渲染结果" className="render-result" />
              ) : (
                <>
                  <img src={templateUrl} alt="模板" className="template-image" />
                  <div className="view-overlay">
                    {modelDetails && modelDetails.views.filter(v => !v.skip).map(view => {
                      const file = images[view.name]
                      if (!file) return null
                      const imgUrl = URL.createObjectURL(file)
                      const adj = adjustments[view.name] || { scale: 100, rotate: 0, offsetX: 0, offsetY: 0, flipH: false }
                      const isSelected = selectedView === view.name
                      return (
                        <div key={view.name}
                          className={`view-overlay-region ${isSelected ? 'selected' : ''}`}
                          style={{
                            left: `${(view.x / modelDetails.width) * 100}%`,
                            top: `${(view.y / modelDetails.height) * 100}%`,
                            width: `${(view.w / modelDetails.width) * 100}%`,
                            height: `${(view.h / modelDetails.height) * 100}%`,
                            borderColor: isSelected ? '#fff' : VIEW_COLORS[view.name],
                            zIndex: isSelected ? 10 : 1,
                            transform: `scale(${adj.scale / 100})`,
                            transformOrigin: 'center center',
                          }}
                          onMouseDown={e => handleOverlayMouseDown(e, view.name)}
                          onWheel={e => handleOverlayWheel(e, view.name)}>
                          <div className="overlay-image-wrapper"
                            style={{
                              transform: `rotate(${adj.rotate || 0}deg) translate(${adj.offsetX || 0}px, ${adj.offsetY || 0}px)`,
                              transformOrigin: 'center center',
                            }}>
                            <img src={imgUrl} alt={view.name}
                              className="overlay-image"
                              style={{
                                width: '100%', height: '100%',
                                objectFit: 'cover',
                                opacity: isSelected ? 0.85 : 0.6,
                                transform: adj.flipH ? 'scaleX(-1)' : 'none',
                                cursor: 'grab',
                              }}
                            />
                          </div>
                          <div className="region-label" style={{ background: VIEW_COLORS[view.name] }}>
                            {VIEW_LABELS[view.name]}
                          </div>
                        </div>
                      )
                    })}
                    {/* Click empty area to deselect */}
                    {!resultUrl && (
                      <div className="overlay-click-catcher" onClick={() => setSelectedView(null)} />
                    )}
                  </div>
                </>
              )}
            </div>

            <div className="actions">
              <button className="btn btn-primary"
                onClick={handleRender}
                disabled={rendering || Object.keys(images).length === 0}>
                {rendering ? '⏳ 渲染中...' : '🎨 生成 Wrap'}
              </button>
              {resultUrl && (
                <button className="btn btn-success" onClick={handleDownload}>
                  ⬇️ 下载 PNG
                </button>
              )}
            </div>
            {modelDetails && (
              <div className="model-info">
                <small>已上传: {Object.keys(images).length}/{modelDetails.views_count}个视图</small>
              </div>
            )}
          </section>
        </div>
      </main>
    </div>
  )
}

function UploadBox({ viewName, label, color, file, onUpload, onDrop, onClear, dragOver, setDragOver, adjustments }) {
  const inputRef = useRef(null)
  const previewUrl = file ? URL.createObjectURL(file) : null

  return (
    <div className={`upload-box ${dragOver === viewName ? 'drag-over' : ''} ${file ? 'has-file' : ''}`}
      style={{ borderColor: color }}
      onDragOver={e => { e.preventDefault(); setDragOver(viewName) }}
      onDragLeave={() => setDragOver(null)}
      onDrop={e => onDrop(e, viewName)}
      onClick={() => !file && inputRef.current?.click()}>
      <input ref={inputRef} type="file" accept="image/*"
        style={{ display: 'none' }}
        onChange={e => onUpload(viewName, e.target.files[0])} />
      <div className="upload-label" style={{ background: color }}>{label}</div>
      {previewUrl ? (
        <div className="upload-preview">
          <img src={previewUrl} alt={label}
            style={{ transform: adjustments?.flipH ? 'scaleX(-1)' : 'none' }} />
          <button className="upload-remove" onClick={e => { e.stopPropagation(); onClear() }}>✕</button>
          <div className="upload-badge">✓</div>
        </div>
      ) : (
        <div className="upload-placeholder">
          <span className="upload-icon">📷</span>
          <span className="upload-text">点击或拖拽上传</span>
        </div>
      )}
    </div>
  )
}
