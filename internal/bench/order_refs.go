package bench

import "cmp"

type OrderRefContract struct {
	MetadataField string
}

func CleanupOrderRefContract(field string) OrderRefContract {
	return OrderRefContract{MetadataField: cmp.Or(field, MetadataCleanupOrdersKey)}
}

func (c OrderRefContract) Field() string {
	return cmp.Or(c.MetadataField, MetadataCleanupOrdersKey)
}

func (c OrderRefContract) FromMetadata(metadata map[string]any) []OrderRef {
	return OrderRefsFromMetadata(metadata, c.Field())
}

func (c OrderRefContract) FromSample(sample Sample) []OrderRef {
	if len(sample.OrderRefs) > 0 {
		return sample.OrderRefs
	}
	return c.FromMetadata(sample.Metadata)
}

func (c OrderRefContract) PutMetadata(metadata map[string]any, refs []OrderRef) map[string]any {
	if len(refs) == 0 {
		return metadata
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata[c.Field()] = OrderRefsToMetadata(refs)
	return metadata
}
