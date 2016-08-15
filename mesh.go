package main

import (
	"github.com/gnewton/pubmedSqlStructs"
	"github.com/gnewton/pubmedstruct"
)

func makeMeshDescriptors(mhs []*pubmedstruct.MeshHeading) []*pubmedSqlStructs.MeshDescriptor {
	meshDescriptors := make([]*pubmedSqlStructs.MeshDescriptor, len(mhs))

	for i, mh := range mhs {
		newMeshDescriptor := new(pubmedSqlStructs.MeshDescriptor)
		newMeshDescriptor.MajorTopic = (mh.DescriptorName.Attr_MajorTopicYN == "Y")
		newMeshDescriptor.Type = mh.DescriptorName.Attr_Type
		newMeshDescriptor.Name = mh.DescriptorName.Text
		newMeshDescriptor.Qualifiers = makeQualifiers(mh.QualifierName)
		newMeshDescriptor.UI = mh.DescriptorName.Attr_UI

		meshDescriptors[i] = newMeshDescriptor
	}
	return meshDescriptors
}

func makeQualifiers(qns []*pubmedstruct.QualifierName) []*pubmedSqlStructs.MeshQualifier {
	qualifiers := make([]*pubmedSqlStructs.MeshQualifier, len(qns))

	for i, q := range qns {
		mq := new(pubmedSqlStructs.MeshQualifier)
		mq.Name = q.Text
		mq.MajorTopic = (q.Attr_MajorTopicYN == "Y")
		mq.UI = q.Attr_UI
		qualifiers[i] = mq
	}
	return qualifiers
}