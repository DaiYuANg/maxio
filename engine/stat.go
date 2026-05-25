package engine

import "context"

// StatObject returns object layout metadata without reading object data.
func (e *Engine) StatObject(ctx context.Context, bucket, key string) (ObjectInfo, error) {
	_ = ctx
	layout, err := e.getLayout(bucket, key)
	if err != nil {
		return ObjectInfo{}, err
	}
	return e.objectInfoFromLayout(layout), nil
}
